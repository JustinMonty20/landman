package fema

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"os"
	"testing"
)

func TestArchiveRejectsCorruptMember(t *testing.T) {
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	member, err := writer.CreateHeader(&zip.FileHeader{Name: "STUDY_INFO.DBF", Method: zip.Store})
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("fixture payload")
	if _, err := member.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	archive := buf.Bytes()
	offset := bytes.Index(archive, payload)
	if offset < 0 {
		t.Fatal("fixture is not stored uncompressed")
	}
	archive[offset] ^= 1
	if _, err := decodeArchive(context.Background(), bytes.NewReader(archive), int64(len(archive))); !errors.Is(err, zip.ErrChecksum) {
		t.Fatalf("expected ZIP checksum failure, got %v", err)
	}
}

func TestSavedUnionArchive(t *testing.T) {
	filename := os.Getenv("FEMA_TEST_ARCHIVE")
	if filename == "" {
		t.Skip("set FEMA_TEST_ARCHIVE to the saved official ZIP")
	}
	file, err := os.Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	release, err := decodeArchive(context.Background(), file, info.Size())
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("decoded %d flood zones and %d panels", len(release.Zones), len(release.Panels))
}
