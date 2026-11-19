package fema

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/xy"
	"github.com/twpayne/go-geom/xy/lineintersector"
	"github.com/twpayne/go-geom/xy/location"
)

const nad83 = `GEOGCS["GCS_North_American_1983",DATUM["D_North_American_1983",SPHEROID["GRS_1980",6378137.0,298.257222101]],PRIMEM["Greenwich",0.0],UNIT["Degree",0.0174532925199433]]`
const maxMember = 128 << 20

type Zone struct {
	ID, Code, Subtype, SFHA string
	Shape                   *geom.MultiPolygon
}
type Panel struct {
	ID, Number string
	Effective  *time.Time
	Shape      *geom.MultiPolygon
}
type Release struct {
	Zones                            []Zone
	Panels                           []Panel
	IndexEffective, DatabaseRevision *time.Time
}

func decodeArchive(ctx context.Context, reader io.ReaderAt, size int64) (Release, error) {
	z, err := zip.NewReader(reader, size)
	if err != nil {
		return Release{}, err
	}
	if len(z.File) > 512 {
		return Release{}, errors.New("too many FEMA ZIP members")
	}
	files := make(map[string]*zip.File)
	var expanded uint64
	for _, f := range z.File {
		if err := ctx.Err(); err != nil {
			return Release{}, err
		}
		if f.FileInfo().IsDir() {
			continue
		}
		name := strings.ToUpper(f.Name)
		if path.Base(name) != name || strings.ContainsAny(name, `\:`) || name == "." || name == ".." || files[name] != nil || !f.Mode().IsRegular() {
			return Release{}, fmt.Errorf("invalid FEMA ZIP member %q", f.Name)
		}
		if f.UncompressedSize64 > maxMember || expanded+f.UncompressedSize64 > 512<<20 {
			return Release{}, errors.New("FEMA ZIP expansion limit exceeded")
		}
		expanded += f.UncompressedSize64
		files[name] = f
		if err := checkZIP(ctx, f); err != nil {
			return Release{}, fmt.Errorf("FEMA ZIP member %s: %w", f.Name, err)
		}
	}
	read := func(name string) ([]byte, error) {
		f := files[name]
		if f == nil {
			return nil, fmt.Errorf("missing FEMA archive member %s", name)
		}
		rc, e := f.Open()
		if e != nil {
			return nil, e
		}
		defer rc.Close()
		return io.ReadAll(io.LimitReader(contextReader{ctx, rc}, maxMember+1))
	}
	study, err := read("STUDY_INFO.DBF")
	if err != nil {
		return Release{}, err
	}
	rows, err := dbfRows(study, "DFIRM_ID", "INDX_EFFDT", "DBREV_DT")
	if err != nil {
		return Release{}, fmt.Errorf("study metadata: %w", err)
	}
	if len(rows) != 1 || rows[0][0] != "37179C" {
		return Release{}, errors.New("FEMA archive does not identify the Union County study")
	}
	var result Release
	if result.IndexEffective, err = femaDate(rows[0][1]); err != nil {
		return Release{}, err
	}
	if result.DatabaseRevision, err = femaDate(rows[0][2]); err != nil {
		return Release{}, err
	}
	layer := func(name string, fields ...string) ([][]string, []*geom.MultiPolygon, error) {
		projection, e := read(name + ".PRJ")
		if e != nil {
			return nil, nil, e
		}
		if strings.TrimSpace(string(projection)) != nad83 {
			return nil, nil, fmt.Errorf("unsupported %s spatial reference", name)
		}
		attr, e := read(name + ".DBF")
		if e != nil {
			return nil, nil, e
		}
		values, e := dbfRows(attr, fields...)
		if e != nil {
			return nil, nil, e
		}
		raw, e := read(name + ".SHP")
		if e != nil {
			return nil, nil, e
		}
		index, e := read(name + ".SHX")
		if e != nil {
			return nil, nil, e
		}
		shapes, e := shapes(ctx, raw, index, len(values))
		return values, shapes, e
	}
	zones, shape, err := layer("S_FLD_HAZ_AR", "DFIRM_ID", "FLD_AR_ID", "FLD_ZONE", "ZONE_SUBTY", "SFHA_TF")
	if err != nil {
		return Release{}, fmt.Errorf("flood zones: %w", err)
	}
	seen := map[string]bool{}
	for i, r := range zones {
		if r[0] != "37179C" || r[1] == "" || seen[r[1]] {
			return Release{}, fmt.Errorf("invalid flood zone identity at record %d", i+1)
		}
		seen[r[1]] = true
		result.Zones = append(result.Zones, Zone{r[1], r[2], r[3], r[4], shape[i]})
	}
	panels, shape, err := layer("S_FIRM_PAN", "DFIRM_ID", "FIRM_ID", "FIRM_PAN", "EFF_DATE")
	if err != nil {
		return Release{}, fmt.Errorf("panels: %w", err)
	}
	clear(seen)
	for i, r := range panels {
		if r[0] != "37179C" || r[1] == "" || r[2] == "" || seen[r[1]] {
			return Release{}, fmt.Errorf("invalid panel identity at record %d", i+1)
		}
		seen[r[1]] = true
		date, e := femaDate(r[3])
		if e != nil {
			return Release{}, e
		}
		result.Panels = append(result.Panels, Panel{r[1], r[2], date, shape[i]})
	}
	return result, nil
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}
func checkZIP(ctx context.Context, f *zip.File) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	n, err := io.Copy(io.Discard, io.LimitReader(contextReader{ctx, rc}, int64(f.UncompressedSize64)+1))
	if err != nil {
		return err
	}
	if uint64(n) != f.UncompressedSize64 {
		return errors.New("ZIP member size mismatch")
	}
	return nil
}

func femaDate(value string) (*time.Time, error) {
	if value == "" || value == "00000000" || value == "99990909" {
		return nil, nil
	}
	t, err := time.Parse("20060102", value)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func dbfRows(raw []byte, names ...string) ([][]string, error) {
	if len(raw) < 33 {
		return nil, errors.New("short DBF header")
	}
	n := int(binary.LittleEndian.Uint32(raw[4:8]))
	h := int(binary.LittleEndian.Uint16(raw[8:10]))
	size := int(binary.LittleEndian.Uint16(raw[10:12]))
	if n <= 0 || n > 100000 || h < 33 || h > len(raw) || size < 2 || (h-33)%32 != 0 || raw[h-1] != 13 || n > (len(raw)-h)/size {
		return nil, errors.New("invalid DBF dimensions")
	}
	end := h + n*size
	if end != len(raw) && (end+1 != len(raw) || raw[end] != 0x1a) {
		return nil, errors.New("unexpected DBF record count")
	}
	type span struct{ start, end int }
	fields := map[string]span{}
	offset := 1
	for i := 32; i < h-1; i += 32 {
		d := raw[i : i+32]
		key := string(bytes.TrimRight(d[:11], "\x00"))
		width := int(d[16])
		if _, ok := fields[key]; ok || key == "" || width == 0 || offset+width > size {
			return nil, errors.New("invalid DBF field")
		}
		fields[key] = span{offset, offset + width}
		offset += width
	}
	if offset != size {
		return nil, errors.New("DBF record width mismatch")
	}
	columns := make([]span, len(names))
	for i, name := range names {
		field, ok := fields[name]
		if !ok {
			return nil, fmt.Errorf("missing DBF field %s", name)
		}
		columns[i] = field
	}
	out := make([][]string, 0, n)
	for i := 0; i < n; i++ {
		rec := raw[h+i*size : h+(i+1)*size]
		if rec[0] != ' ' {
			return nil, errors.New("inactive DBF record")
		}
		row := make([]string, len(columns))
		for j, c := range columns {
			value := bytes.TrimSpace(rec[c.start:c.end])
			if !utf8.Valid(value) || bytes.ContainsRune(value, 0) {
				return nil, errors.New("invalid DBF text")
			}
			row[j] = string(value)
		}
		out = append(out, row)
	}
	return out, nil
}

func shapes(ctx context.Context, raw, index []byte, count int) ([]*geom.MultiPolygon, error) {
	be, le := binary.BigEndian, binary.LittleEndian
	header := func(b []byte) bool {
		return len(b) >= 100 && be.Uint32(b) == 9994 && int64(be.Uint32(b[24:]))*2 == int64(len(b)) && le.Uint32(b[28:]) == 1000 && le.Uint32(b[32:]) == 5
	}
	if !header(raw) || !header(index) || len(index) != 100+8*count {
		return nil, errors.New("invalid polygon shape or index header")
	}
	out := make([]*geom.MultiPolygon, 0, count)
	start := 100
	for i := 0; i < count; i++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		idx := index[100+8*i:]
		length := int64(be.Uint32(idx[4:])) * 2
		if int64(be.Uint32(idx))*2 != int64(start) || length < 44 || len(raw)-start < 8 || length > int64(len(raw)-start-8) || be.Uint32(raw[start:]) != uint32(i+1) || be.Uint32(raw[start+4:]) != be.Uint32(idx[4:]) {
			return nil, fmt.Errorf("shape index mismatch at record %d", i+1)
		}
		stop := start + 8 + int(length)
		shape, err := polygon(raw[start+8 : stop])
		if err != nil {
			return nil, fmt.Errorf("shape %d: %w", i+1, err)
		}
		out = append(out, shape)
		start = stop
	}
	if start != len(raw) {
		return nil, errors.New("extra shapes beyond DBF count")
	}
	return out, nil
}

func polygon(b []byte) (*geom.MultiPolygon, error) {
	le := binary.LittleEndian
	if le.Uint32(b) != 5 {
		return nil, errors.New("expected XY polygon")
	}
	parts, points := int(le.Uint32(b[36:])), int(le.Uint32(b[40:]))
	if parts < 1 || parts > 10000 || points < 4 || points > 1000000 || 44+4*parts+16*points != len(b) {
		return nil, errors.New("invalid polygon dimensions")
	}
	coords := make([]float64, points*2)
	for i := range coords {
		x := math.Float64frombits(le.Uint64(b[44+4*parts+i*8:]))
		bound := 180.0
		if i%2 == 1 {
			bound = 90
		}
		if math.IsNaN(x) || math.Abs(x) > bound {
			return nil, errors.New("invalid longitude or latitude")
		}
		coords[i] = x
	}
	type shell struct {
		ring  []float64
		holes [][]float64
		area  float64
	}
	var shells []shell
	var holes [][]float64
	for i, start := 0, 0; i < parts; i++ {
		if int(le.Uint32(b[44+4*i:])) != start {
			return nil, errors.New("invalid polygon part offset")
		}
		end := points
		if i+1 < parts {
			end = int(le.Uint32(b[48+4*i:]))
		}
		if end-start < 4 || end > points {
			return nil, errors.New("short or invalid polygon ring")
		}
		ring := coords[2*start : 2*end]
		start = end
		if ring[0] != ring[len(ring)-2] || ring[1] != ring[len(ring)-1] {
			return nil, errors.New("open polygon ring")
		}
		if xy.IsRingCounterClockwise(geom.XY, ring) {
			holes = append(holes, ring)
		} else {
			area := geom.NewPolygonFlat(geom.XY, ring, []int{len(ring)}).Area()
			shells = append(shells, shell{ring: ring, area: math.Abs(area)})
		}
	}
	for _, hole := range holes {
		owner := -1
		for i, s := range shells {
			if (owner < 0 || s.area < shells[owner].area) && inside(hole, s.ring) {
				owner = i
			}
		}
		if owner < 0 {
			return nil, errors.New("polygon hole lacks a containing shell")
		}
		shells[owner].holes = append(shells[owner].holes, hole)
	}
	multi := geom.NewMultiPolygon(geom.XY).SetSRID(4269)
	if len(shells) == 0 {
		return nil, errors.New("polygon has no shell")
	}
	for _, s := range shells {
		rings := [][]float64{s.ring}
		rings = append(rings, s.holes...)
		var flat []float64
		var ends []int
		for _, r := range rings {
			flat = append(flat, r...)
			ends = append(ends, len(flat))
		}
		if err := multi.Push(geom.NewPolygonFlat(geom.XY, flat, ends)); err != nil {
			return nil, err
		}
	}
	return multi, nil
}

func inside(hole, shell []float64) bool {
	if xy.LocatePointInRing(geom.XY, geom.Coord(hole[:2]), shell) != location.Interior {
		return false
	}
	for i := 0; i < len(hole)-2; i += 2 {
		minX, maxX := min(hole[i], hole[i+2]), max(hole[i], hole[i+2])
		minY, maxY := min(hole[i+1], hole[i+3]), max(hole[i+1], hole[i+3])
		for j := 0; j < len(shell)-2; j += 2 {
			if minX > max(shell[j], shell[j+2]) || maxX < min(shell[j], shell[j+2]) || minY > max(shell[j+1], shell[j+3]) || maxY < min(shell[j+1], shell[j+3]) {
				continue
			}
			intersection := lineintersector.LineIntersectsLine(lineintersector.RobustLineIntersector{}, geom.Coord(hole[i:i+2]), geom.Coord(hole[i+2:i+4]), geom.Coord(shell[j:j+2]), geom.Coord(shell[j+2:j+4]))
			if intersection.HasIntersection() {
				return false
			}
		}
	}
	return true
}
