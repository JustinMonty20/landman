package parcelevidence

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Receipt names one immutable response in request order. Path is relative to the root.
type Receipt struct {
	Path        string    `json:"path"`
	Request     string    `json:"request"`
	RetrievedAt time.Time `json:"retrieved_at"`
	Bytes       int       `json:"bytes"`
	SHA256      string    `json:"sha256"`
	DeclaredCRS int       `json:"declared_crs,omitempty"`
	Validated   bool      `json:"validated"`
}

type Manifest struct {
	ArtifactHash        string    `json:"artifact_hash"`
	ArtifactHashVersion string    `json:"artifact_hash_version"`
	Version             int       `json:"version"`
	Source              string    `json:"source"`
	Kind                string    `json:"kind"`
	ID                  string    `json:"id"`
	CreatedAt           time.Time `json:"created_at"`
	Count               int       `json:"count"`
	Receipts            []Receipt `json:"receipts"`
}

// Snapshot is a concrete private local disk capture. A manifest is written only on completion.
type Snapshot struct {
	root     string
	Manifest Manifest
}

func New(root, kind string) (*Snapshot, error) {
	if root == "" {
		return nil, fmt.Errorf("parcel evidence directory required")
	}
	if kind != "full_scan" && kind != "reprocess" {
		return nil, fmt.Errorf("invalid capture kind")
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("parcel evidence directory must be private")
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return nil, err
	}
	name := hex.EncodeToString(id)
	dir := filepath.Join(root, name)
	if err := os.Mkdir(dir, 0700); err != nil {
		return nil, err
	}
	if err := syncDirectory(root); err != nil {
		return nil, err
	}
	return &Snapshot{root: root, Manifest: Manifest{Version: 1, Source: "union_county_gis", Kind: kind, ID: name, CreatedAt: time.Now().UTC()}}, nil
}

func safeID(id string) bool {
	if len(id) != 32 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

// Save stores original bytes before a response can be normalized. A receipt is
// immutable and cannot be replaced by a later repair or retry.
func (s *Snapshot) Save(request string, body []byte, crs int) error {
	if !safeID(s.Manifest.ID) || len(body) == 0 || len(body) > 32<<20 {
		return fmt.Errorf("invalid evidence response")
	}
	if crs != 0 && crs != 3857 && crs != 102100 {
		return fmt.Errorf("unsupported source CRS %d", crs)
	}
	name := fmt.Sprintf("%06d.json", len(s.Manifest.Receipts))
	path := filepath.Join(s.root, s.Manifest.ID, name)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	n, writeErr := f.Write(body)
	syncErr := f.Sync()
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	if syncErr != nil {
		return syncErr
	}
	if closeErr != nil {
		return closeErr
	}
	if n != len(body) {
		return fmt.Errorf("short evidence write")
	}
	if err := syncDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	hash := sha256.Sum256(body)
	s.Manifest.Receipts = append(s.Manifest.Receipts, Receipt{Path: name, Request: request, RetrievedAt: time.Now().UTC(), Bytes: len(body), SHA256: hex.EncodeToString(hash[:]), DeclaredCRS: crs, Validated: true})
	return nil
}

func (s *Snapshot) Complete(count int) error {
	if count < 0 || len(s.Manifest.Receipts) == 0 {
		return fmt.Errorf("incomplete evidence capture")
	}
	s.Manifest.Count = count
	s.Manifest.ArtifactHashVersion = "ordered-response-bytes-v1"
	hash, err := s.artifactHash()
	if err != nil {
		return err
	}
	s.Manifest.ArtifactHash = hash
	if err := s.Verify(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.Manifest, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(s.root, s.Manifest.ID, "manifest.json")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(data)
	syncErr := f.Sync()
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	if syncErr != nil {
		return syncErr
	}
	if closeErr != nil {
		return closeErr
	}
	return syncDirectory(filepath.Dir(path))
}

func Open(root, id string) (*Snapshot, error) {
	if !safeID(id) {
		return nil, fmt.Errorf("invalid snapshot ID")
	}
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return nil, err
	}
	if !rootInfo.IsDir() || rootInfo.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("parcel evidence directory must be private")
	}
	info, err := os.Lstat(filepath.Join(root, id))
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("snapshot directory must be private")
	}
	data, err := readPrivateFile(filepath.Join(root, id, "manifest.json"), 8<<20)
	if err != nil {
		return nil, err
	}
	var m Manifest
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&m); err != nil {
		return nil, err
	}
	if m.ID != id || m.Version != 1 || m.Source != "union_county_gis" || m.Count < 0 || !validDigest(m.ArtifactHash) {
		return nil, fmt.Errorf("invalid manifest identity")
	}
	s := &Snapshot{root: root, Manifest: m}
	return s, s.Verify()
}

func (s *Snapshot) artifactHash() (string, error) {
	h := sha256.New()
	for i := range s.Manifest.Receipts {
		body, err := s.Read(i)
		if err != nil {
			return "", err
		}
		if _, err = h.Write([]byte(fmt.Sprintf("%d:", len(body)))); err != nil {
			return "", err
		}
		if _, err = h.Write(body); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (s *Snapshot) Verify() error {
	if !safeID(s.Manifest.ID) || len(s.Manifest.Receipts) == 0 {
		return fmt.Errorf("invalid evidence manifest")
	}
	for i, receipt := range s.Manifest.Receipts {
		if receipt.Path != fmt.Sprintf("%06d.json", i) || !receipt.Validated || strings.Contains(receipt.Request, "\n") || (receipt.DeclaredCRS != 3857 && receipt.DeclaredCRS != 102100 && !(receipt.DeclaredCRS == 0 && strings.Contains(receipt.Request, "returnCountOnly=true"))) {
			return fmt.Errorf("invalid receipt %d", i)
		}
		if _, err := s.Read(i); err != nil {
			return err
		}
	}
	if s.Manifest.ArtifactHash != "" {
		if s.Manifest.ArtifactHashVersion != "ordered-response-bytes-v1" {
			return fmt.Errorf("unknown artifact hash version")
		}
		hash, err := s.artifactHash()
		if err != nil {
			return err
		}
		if hash != s.Manifest.ArtifactHash {
			return fmt.Errorf("artifact hash mismatch")
		}
	}
	return nil
}

func (s *Snapshot) Read(index int) ([]byte, error) {
	if index < 0 || index >= len(s.Manifest.Receipts) {
		return nil, fmt.Errorf("receipt out of range")
	}
	receipt := s.Manifest.Receipts[index]
	if receipt.Path != fmt.Sprintf("%06d.json", index) {
		return nil, fmt.Errorf("invalid evidence path")
	}
	if receipt.Bytes <= 0 || receipt.Bytes > 32<<20 || !validDigest(receipt.SHA256) {
		return nil, fmt.Errorf("invalid evidence receipt size or digest")
	}
	body, err := readPrivateFile(filepath.Join(s.root, s.Manifest.ID, receipt.Path), 32<<20)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(body)
	if len(body) != receipt.Bytes || hex.EncodeToString(sum[:]) != receipt.SHA256 {
		return nil, fmt.Errorf("evidence checksum mismatch at %d", index)
	}
	return body, nil
}

func validDigest(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			if c < 'a' || c > 'f' {
				return false
			}
		}
	}
	return true
}

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func readPrivateFile(path string, limit int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() <= 0 || info.Size() > limit {
		return nil, fmt.Errorf("evidence file must be private regular bounded file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !os.SameFile(info, opened) || !opened.Mode().IsRegular() {
		return nil, fmt.Errorf("evidence file changed during open")
	}
	return io.ReadAll(io.LimitReader(file, limit+1))
}
