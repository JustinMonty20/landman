package union

import (
	"archive/zip"
	"bytes"
	"compress/zlib"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/JustinMonty20/landman/internal/taxartifact"
	"github.com/JustinMonty20/landman/internal/taxsource"
)

const pipelineTestURL = "https://www.unioncountync.gov/home/showdocument?id=4784"

func pipelineFixture(t *testing.T, path string) *PipelineList {
	t.Helper()
	store, err := taxartifact.NewLocal(filepath.Join(t.TempDir(), "private"))
	if err != nil {
		t.Fatal(err)
	}
	list, err := NewPipelineList(path, pipelineTestURL, store)
	if err != nil {
		t.Fatal(err)
	}
	return list
}
func TestPipelineFixtures(t *testing.T) {
	for _, tc := range []struct {
		file, key, owner string
		cents            int64
		count            int
	}{
		{"pipeline.pdf", "02239077", "Unknown Heirs of Freddie Sowell", 15660000, 1},
		{"pipeline.pptx", "07027002J80", "Test Owner", 5000, 4},
	} {
		t.Run(tc.file, func(t *testing.T) {
			list := pipelineFixture(t, filepath.Join("testdata", "tax", tc.file))
			var rows []PipelineProperty
			meta, err := list.Read(context.Background(), func(p PipelineProperty) error { rows = append(rows, p); return nil })
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != tc.count || rows[0].ParcelKey != tc.key || rows[0].OwnerName != tc.owner || rows[0].TaxValue == nil || *rows[0].TaxValue != tc.cents {
				t.Fatalf("rows=%+v", rows)
			}
			if tc.file == "pipeline.pptx" && (rows[1].ParcelKey != "07027002J90" || rows[2].ParcelKey != "09125042" || rows[3].ParcelKey != "09125043" || rows[3].Township != "Monroe") {
				t.Fatalf("PPTX runs, table, descr, notes: %+v", rows)
			}
			if !meta.Artifact.Verified() || meta.SHA256 != meta.Artifact.SHA256() {
				t.Fatalf("evidence=%+v", meta)
			}
			skipped, err := list.Skipped(context.Background(), meta)
			if err != nil || skipped != 0 {
				t.Fatalf("skipped=%d err=%v", skipped, err)
			}
		})
	}
}
func TestPipelineRejectedInput(t *testing.T) {
	root := t.TempDir()
	store, err := taxartifact.NewLocal(filepath.Join(root, "private"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewPipelineList("", pipelineTestURL, store); err == nil {
		t.Fatal("missing path accepted")
	}
	if _, err := NewPipelineList("file", "", store); err == nil {
		t.Fatal("missing URL accepted")
	}
	if _, err := NewPipelineList("file", "http://example.com", store); err == nil {
		t.Fatal("insecure URL accepted")
	}
	if _, err := NewPipelineList("file", pipelineTestURL, nil); err == nil {
		t.Fatal("missing store accepted")
	}
	for _, tc := range []struct{ name, body string }{{"invalid", "bad"}, {"bad zip", "PKbad"}, {"empty pdf", "%PDF-1.4"}} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(root, tc.name)
			if err := os.WriteFile(path, []byte(tc.body), 0600); err != nil {
				t.Fatal(err)
			}
			list := pipelineFixture(t, path)
			n := 0
			_, err := list.Read(context.Background(), func(PipelineProperty) error { n++; return nil })
			if err == nil || n != 0 {
				t.Fatalf("err=%v yields=%d", err, n)
			}
		})
	}
	missing := pipelineFixture(t, filepath.Join(root, "absent.pdf"))
	if _, err := missing.Read(context.Background(), func(PipelineProperty) error { return nil }); err == nil {
		t.Fatal("missing file accepted")
	}
	picture, err := os.ReadFile(filepath.Join("testdata", "tax", "pipeline.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	picture = bytes.Replace(picture, []byte("Parcel: 02.239.077"), bytes.Repeat([]byte{' '}, len("Parcel: 02.239.077")), 1)
	picturePath := filepath.Join(root, "picture.pdf")
	if err := os.WriteFile(picturePath, picture, 0600); err != nil {
		t.Fatal(err)
	}
	pictureList := pipelineFixture(t, picturePath)
	count := 0
	meta, err := pictureList.Read(context.Background(), func(PipelineProperty) error { count++; return nil })
	if err != nil || count != 0 {
		t.Fatalf("picture-only err=%v count=%d", err, count)
	}
	skipped, err := pictureList.Skipped(context.Background(), meta)
	if err != nil || skipped != 1 {
		t.Fatalf("skipped=%d err=%v", skipped, err)
	}
	list := pipelineFixture(t, filepath.Join("testdata", "tax", "pipeline.pdf"))
	if _, err := list.Read(context.Background(), nil); err == nil {
		t.Fatal("missing yield accepted")
	}
	if _, err := list.Skipped(context.Background(), taxsource.Meta{}); err == nil {
		t.Fatal("unverified artifact accepted")
	}
	want := errors.New("consumer failed")
	if _, err := list.Read(context.Background(), func(PipelineProperty) error { return want }); !errors.Is(err, want) {
		t.Fatalf("yield error=%v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	n := 0
	_, err = list.Read(ctx, func(PipelineProperty) error { n++; return nil })
	if !errors.Is(err, context.Canceled) || n != 0 {
		t.Fatalf("cancellation err=%v yields=%d", err, n)
	}
}
func TestPipelineProperties(t *testing.T) {
	rows, err := pipelineProperties([]string{"Parcel: 02.239.077 Parcel: 07.027.002J80", "Tax Value: $1,234.56", "Acreage: Lot 16 College Grove", "Sale: August 27", "Status: Redeemed/Satisfied"})
	if err != nil || len(rows) != 2 || rows[1].ParcelKey != "07027002J80" || rows[1].SaleOn != nil || !rows[1].Redeemed || rows[1].Acreage != "Lot 16 College Grove" {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
	for _, value := range []string{"12", "bad", "-1.00"} {
		_, err := pipelineProperties([]string{"Parcel: 02.239.077", "Tax Value: " + value})
		if err == nil {
			t.Fatalf("accepted %s", value)
		}
	}
	rows, err = pipelineProperties([]string{"Parcel", "Picture only"})
	if err != nil || len(rows) != 0 {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
}

func TestPipelineCompressedPDFAndAlternatives(t *testing.T) {
	var compressed bytes.Buffer
	writer := zlib.NewWriter(&compressed)
	_, err := writer.Write([]byte("BT [(Parcel: ) 0 (09.125.043)] TJ 0 -18 Td (Owner: Text Layer) Tj ET\n/Span << /ActualText <FEFF0054006F0077006E0073006800690070003A0020004D006F006E0072006F0065> >> BDC\n/Span << /Alt (Acreage: 5.30 acs) >> BDC"))
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "compressed.pdf")
	if err := os.WriteFile(path, pipelineTestPDF(compressed.Bytes(), true), 0600); err != nil {
		t.Fatal(err)
	}
	list := pipelineFixture(t, path)
	var rows []PipelineProperty
	_, err = list.Read(context.Background(), func(row PipelineProperty) error { rows = append(rows, row); return nil })
	if err != nil || len(rows) != 1 || rows[0].ParcelKey != "09125043" || rows[0].Acreage != "5.30 acs" || rows[0].Township != "Monroe" {
		t.Fatalf("compressed PDF rows=%+v err=%v", rows, err)
	}
}

func TestPipelinePictureOnlyPPTX(t *testing.T) {
	var buf bytes.Buffer
	archive := zip.NewWriter(&buf)
	for name, content := range map[string]string{
		"[Content_Types].xml":   "<Types><Override PartName=\"/ppt/presentation.xml\" ContentType=\"application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml\"/></Types>",
		"ppt/presentation.xml":  "<p:presentation xmlns:p=\"http://schemas.openxmlformats.org/presentationml/2006/main\"/>",
		"ppt/slides/slide1.xml": "<p:sld xmlns:p=\"http://schemas.openxmlformats.org/presentationml/2006/main\"><p:pic/></p:sld>",
	} {
		entry, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "picture.pptx")
	if err := os.WriteFile(path, buf.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	list := pipelineFixture(t, path)
	count := 0
	meta, err := list.Read(context.Background(), func(PipelineProperty) error { count++; return nil })
	if err != nil || count != 0 {
		t.Fatalf("read err=%v count=%d", err, count)
	}
	skipped, err := list.Skipped(context.Background(), meta)
	if err != nil || skipped != 1 {
		t.Fatalf("skipped=%d err=%v", skipped, err)
	}
}

func pipelineTestPDF(content []byte, compressed bool) []byte {
	filter := ""
	if compressed {
		filter = " /Filter /FlateDecode"
	}
	objects := [][]byte{
		[]byte("<< /Type /Catalog /Pages 2 0 R >>"),
		[]byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		[]byte("<< /Type /Page /Parent 2 0 R /Contents 4 0 R >>"),
		append([]byte(fmt.Sprintf("<< /Length %d%s >>\nstream\n", len(content), filter)), append(content, []byte("\nendstream")...)...),
	}
	var pdf bytes.Buffer
	pdf.WriteString("%PDF-1.4\n")
	offsets := []int{0}
	for i, object := range objects {
		offsets = append(offsets, pdf.Len())
		fmt.Fprintf(&pdf, "%d 0 obj\n", i+1)
		pdf.Write(object)
		pdf.WriteString("\nendobj\n")
	}
	offset := pdf.Len()
	fmt.Fprintf(&pdf, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for _, entry := range offsets[1:] {
		fmt.Fprintf(&pdf, "%010d 00000 n \n", entry)
	}
	fmt.Fprintf(&pdf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), offset)
	return pdf.Bytes()
}

func TestPipelinePDFTextBlocksAndValidation(t *testing.T) {
	cases := []struct {
		name, content string
		keys          []string
		owners        []string
		cents         []int64
		bad           bool
	}{
		{name: "hex parcel with suffix space", content: "BT <50617263656C3A2030322E3137362E303131203630> Tj 1 0 0 1 0 20 Tm (Owner: First) Tj ET", keys: []string{"02176011 60"}, owners: []string{"First"}},
		{name: "separate properties", content: "BT (Parcel: 02.239.077) Tj 1 0 0 1 0 20 Tm (Owner: First) Tj 1 0 0 1 0 40 Tm (Tax Value: $10.00) Tj 1 0 0 1 0 60 Tm (Parcel: 07.027.002J80) Tj 1 0 0 1 0 80 Tm (Owner: Second) Tj 1 0 0 1 0 100 Tm (Tax Value: $20.00) Tj ET", keys: []string{"02239077", "07027002J80"}, owners: []string{"First", "Second"}, cents: []int64{1000, 2000}},
		{name: "invalid second value", content: "BT (Parcel: 02.239.077) Tj 1 0 0 1 0 20 Tm (Tax Value: $10.00) Tj 1 0 0 1 0 40 Tm (Parcel: 07.027.002J80) Tj 1 0 0 1 0 60 Tm (Tax Value: invalid) Tj ET", bad: true},
		{name: "shared preface", content: "BT (Owner: Common) Tj 1 0 0 1 0 20 Tm (Parcel: 02.239.077) Tj 1 0 0 1 0 40 Tm (Parcel: 07.027.002J80) Tj ET", keys: []string{"02239077", "07027002J80"}, owners: []string{"Common", "Common"}},
		{name: "unsupported text operand", content: "BT (Parcel: 02.239.077) Tj 12 Tj ET", bad: true},
		{name: "image only", content: "q 1 0 0 1 0 0 cm /Im1 Do Q"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "input.pdf")
			if err := os.WriteFile(path, pipelineTestPDF([]byte(tc.content), false), 0600); err != nil {
				t.Fatal(err)
			}
			list := pipelineFixture(t, path)
			var rows []PipelineProperty
			meta, err := list.Read(context.Background(), func(row PipelineProperty) error { rows = append(rows, row); return nil })
			if tc.bad {
				if err == nil || len(rows) != 0 {
					t.Fatalf("invalid file err=%v rows=%+v", err, rows)
				}
				return
			}
			if err != nil || len(rows) != len(tc.keys) {
				t.Fatalf("err=%v rows=%+v", err, rows)
			}
			for i, row := range rows {
				if row.ParcelKey != tc.keys[i] || row.OwnerName != tc.owners[i] {
					t.Fatalf("row %d=%+v", i, row)
				}
				if len(tc.cents) > 0 && (row.TaxValue == nil || *row.TaxValue != tc.cents[i]) {
					t.Fatalf("amount row %d=%+v", i, row)
				}
			}
			if len(rows) == 0 {
				skipped, err := list.Skipped(context.Background(), meta)
				if err != nil || skipped != 1 {
					t.Fatalf("skipped=%d err=%v", skipped, err)
				}
			}
		})
	}
	original := pipelineTestPDF([]byte("BT (Parcel: 02.239.077) Tj ET"), false)
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"truncated", original[:len(original)-12]},
		{"invalid startxref", bytes.Replace(original, []byte("startxref\n"+strconv.Itoa(bytes.Index(original, []byte("xref\n")))), []byte("startxref\n1"), 1)},
		{"wrong length", bytes.Replace(original, []byte("/Length 29"), []byte("/Length 99"), 1)},
		{"unsupported filter", bytes.Replace(original, []byte("/Length 29"), []byte("/Length 29 /Filter /ASCII85Decode"), 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "bad.pdf")
			if err := os.WriteFile(path, tc.data, 0600); err != nil {
				t.Fatal(err)
			}
			list := pipelineFixture(t, path)
			n := 0
			_, err := list.Read(context.Background(), func(PipelineProperty) error { n++; return nil })
			if err == nil || n != 0 {
				t.Fatalf("err=%v yields=%d", err, n)
			}
		})
	}
}

func TestPipelinePPTXParagraphAndContentTypes(t *testing.T) {
	const contentType = `<Types><Override PartName="/ppt/presentation.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"/></Types>`
	const slide = `<p:sld xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><a:p><a:r><a:t>Parcel: 02.</a:t></a:r><a:r><a:t>176.011 60</a:t></a:r></a:p><a:p><a:r><a:t>Owner: Two</a:t></a:r></a:p></p:sld>`
	for _, tc := range []struct {
		name, types, body string
		bad               bool
	}{
		{"paragraph runs", contentType, slide, false},
		{"spoofed content type", `<Types><!-- presentationml.presentation --></Types>`, slide, true},
		{"malformed content types", `<Types><Override ContentType="presentationml.presentation"`, slide, true},
		{"malformed slide", contentType, `<p:sld`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			archive := zip.NewWriter(&buf)
			for name, value := range map[string]string{"[Content_Types].xml": tc.types, "ppt/presentation.xml": `<p:presentation xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"/>`, "ppt/slides/slide1.xml": tc.body} {
				entry, err := archive.Create(name)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := entry.Write([]byte(value)); err != nil {
					t.Fatal(err)
				}
			}
			if err := archive.Close(); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "input.pptx")
			if err := os.WriteFile(path, buf.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			list := pipelineFixture(t, path)
			var rows []PipelineProperty
			_, err := list.Read(context.Background(), func(row PipelineProperty) error { rows = append(rows, row); return nil })
			if tc.bad {
				if err == nil || len(rows) != 0 {
					t.Fatalf("invalid ZIP err=%v rows=%+v", err, rows)
				}
				return
			}
			if err != nil || len(rows) != 1 || rows[0].ParcelKey != "02176011 60" || rows[0].OwnerName != "Two" {
				t.Fatalf("err=%v rows=%+v", err, rows)
			}
		})
	}
}

func TestPipelinePDFEncodingAndMetadata(t *testing.T) {
	cases := []struct {
		name, content string
		key, owner    string
		bad           bool
	}{
		{"UTF16 hex", `BT <FEFF00500061007200630065006C003A002000300032002E003200330039002E003000370037> Tj 1 0 0 1 0 20 Tm (Owner: First) Tj ET`, "02239077", "First", false},
		{"escaped literal", `BT (Parcel: 02.239.077) Tj 1 0 0 1 0 20 Tm (Owner: Fi\162st) Tj ET`, "02239077", "First", false},
		{"invalid UTF16", `BT <FEFF00> Tj ET`, "", "", true},
		{"unsupported encoding", `BT <FF40> Tj ET`, "", "", true},
		{"invalid hex", `BT <GG> Tj ET`, "", "", true},
		{"invalid hex in text array", `BT [(Parcel: 02.239.077) <GG>] TJ ET`, "", "", true},
		{"invalid alternative text", `BT (Parcel: 02.239.077) Tj /Span << /ActualText <GG> >> BDC ET`, "", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "test.pdf")
			if err := os.WriteFile(path, pipelineTestPDF([]byte(tc.content), false), 0600); err != nil {
				t.Fatal(err)
			}
			list := pipelineFixture(t, path)
			var rows []PipelineProperty
			_, err := list.Read(context.Background(), func(row PipelineProperty) error { rows = append(rows, row); return nil })
			if tc.bad {
				if err == nil || len(rows) > 0 {
					t.Fatalf("unsupported text err=%v rows=%+v", err, rows)
				}
				return
			}
			if err != nil || len(rows) != 1 || rows[0].ParcelKey != tc.key || rows[0].OwnerName != tc.owner {
				t.Fatalf("err=%v rows=%+v", err, rows)
			}
		})
	}
}

func TestPipelineCorruptDocumentStructure(t *testing.T) {
	valid := pipelineTestPDF([]byte("BT (Parcel: 02.239.077) Tj ET"), false)
	for _, tc := range []struct {
		name string
		body []byte
	}{
		{"missing catalog", bytes.Replace(valid, []byte("/Type /Catalog"), []byte("/Type /Missing"), 1)},
		{"missing page tree", bytes.Replace(valid, []byte("/Type /Pages"), []byte("/Type /Other"), 1)},
		{"missing page contents", bytes.Replace(valid, []byte("/Contents 4 0 R"), []byte("/Contents 7 0 R"), 1)},
		{"broken compression", bytes.Replace(valid, []byte("/Length 29"), []byte("/Length 29 /Filter /FlateDecode"), 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "invalid.pdf")
			if err := os.WriteFile(path, tc.body, 0600); err != nil {
				t.Fatal(err)
			}
			list := pipelineFixture(t, path)
			n := 0
			_, err := list.Read(context.Background(), func(PipelineProperty) error { n++; return nil })
			if err == nil || n != 0 {
				t.Fatalf("err=%v rows=%d", err, n)
			}
		})
	}
	const contentType = `<Types><Override PartName="/ppt/presentation.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"/></Types>`
	for _, tc := range []struct {
		name  string
		items map[string]string
	}{
		{"missing presentation", map[string]string{"[Content_Types].xml": contentType, "ppt/slides/slide1.xml": `<p:sld xmlns:p="urn:p"/>`}},
		{"invalid presentation", map[string]string{"[Content_Types].xml": contentType, "ppt/presentation.xml": `<invalid/>`, "ppt/slides/slide1.xml": `<p:sld xmlns:p="urn:p"/>`}},
		{"missing slide one", map[string]string{"[Content_Types].xml": contentType, "ppt/presentation.xml": `<p:presentation xmlns:p="urn:p"/>`, "ppt/slides/slide2.xml": `<p:sld xmlns:p="urn:p"/>`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			z := zip.NewWriter(&buf)
			for name, body := range tc.items {
				w, err := z.Create(name)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := w.Write([]byte(body)); err != nil {
					t.Fatal(err)
				}
			}
			if err := z.Close(); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "bad.pptx")
			if err := os.WriteFile(path, buf.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			list := pipelineFixture(t, path)
			n := 0
			_, err := list.Read(context.Background(), func(PipelineProperty) error { n++; return nil })
			if err == nil || n != 0 {
				t.Fatalf("err=%v rows=%d", err, n)
			}
		})
	}
}
