package union

import (
	"archive/zip"
	"bytes"
	"compress/zlib"
	"context"
	"errors"
	"os"
	"path/filepath"
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
	picture = bytes.Replace(picture, []byte("Parcel: 02.239.077"), []byte("Image only           "), 1)
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
	var pdf bytes.Buffer
	pdf.WriteString("%PDF-1.4\n1 0 obj << /Type /Page /Contents [2 0 R] >> endobj\n2 0 obj << /Filter /FlateDecode >>\nstream\n")
	pdf.Write(compressed.Bytes())
	pdf.WriteString("\nendstream\nendobj\n%%EOF")
	path := filepath.Join(t.TempDir(), "compressed.pdf")
	if err := os.WriteFile(path, pdf.Bytes(), 0600); err != nil {
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
		"[Content_Types].xml":   "<Types><Override ContentType=\"application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml\"/></Types>",
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
