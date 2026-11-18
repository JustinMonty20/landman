// Package taxartifact retains exact source bytes before tax rows are parsed.
package taxartifact

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"strings"
)

// Artifact is a portable, content-addressed reference to exact source bytes.
type Artifact struct {
	key      string
	size     int64
	sha256   [32]byte
	mode     string
	verified bool
}

// Key identifies the retained bytes without a local path or an S3 URL.
func (a Artifact) Key() string { return a.key }

// Size is the number of retained bytes.
func (a Artifact) Size() int64 { return a.size }

// SHA256 is the digest of the retained bytes.
func (a Artifact) SHA256() [32]byte { return a.sha256 }

// Mode identifies the backend that verified the stored bytes.
func (a Artifact) Mode() string { return a.mode }

// Verified reports whether Put checked the stored bytes before it returned.
func (a Artifact) Verified() bool { return a.verified }

// Reference reconstructs an unverified artifact from a saved import record.
func Reference(key string, size int64, digest [32]byte) (Artifact, error) {
	a := Artifact{key: key, size: size, sha256: digest}
	return a, a.Valid()
}

// Store has one selected backend per run. Both backends retain immutable versions.
type Store interface {
	Put(context.Context, string, io.Reader) (Artifact, error)
	Get(context.Context, Artifact) (io.ReadCloser, error)
	Mode() string
}

// VerifiedReference checks stored bytes before granting a reconstructed reference
// the same publication authority as a fresh Put result.
func VerifiedReference(ctx context.Context, store Store, artifact Artifact) (Artifact, error) {
	if err := Verify(ctx, store, artifact); err != nil {
		return Artifact{}, err
	}
	artifact.mode = store.Mode()
	artifact.verified = true
	return artifact, nil
}

// Verify reads all bytes and checks their length and SHA-256 before publication.
func Verify(ctx context.Context, store Store, artifact Artifact) error {
	body, err := store.Get(ctx, artifact)
	if err != nil {
		return err
	}
	_, readErr := io.Copy(io.Discard, body)
	closeErr := body.Close()
	return errors.Join(readErr, closeErr)
}

func validSource(source string) bool {
	if source == "" {
		return false
	}
	for i, c := range source {
		if (c >= 'a' && c <= 'z') || (i > 0 && c >= '0' && c <= '9') || (i > 0 && (c == '_' || c == '-')) {
			continue
		}
		return false
	}
	return true
}

func artifactKey(source string, digest [32]byte) string {
	return "tax/" + source + "/sha256/" + hex.EncodeToString(digest[:])
}

// Valid reports whether the artifact reference has a portable content key.
func (a Artifact) Valid() error { return validateArtifact(a) }

func validateArtifact(a Artifact) error {
	parts := strings.Split(a.key, "/")
	if len(parts) != 4 || parts[0] != "tax" || !validSource(parts[1]) || a.size < 0 || a.key != artifactKey(parts[1], a.sha256) {
		return fmt.Errorf("invalid tax artifact reference")
	}
	return nil
}

func stage(ctx context.Context, directory, source string, body io.Reader) (*os.File, Artifact, error) {
	if !validSource(source) || body == nil {
		return nil, Artifact{}, errors.New("invalid tax source or body")
	}
	if err := ctx.Err(); err != nil {
		return nil, Artifact{}, err
	}
	file, err := os.CreateTemp(directory, ".tax-stage-*")
	if err != nil {
		return nil, Artifact{}, err
	}
	failure := func(err error) (*os.File, Artifact, error) {
		file.Close()
		os.Remove(file.Name())
		return nil, Artifact{}, err
	}
	checksum := sha256.New()
	size, err := io.Copy(io.MultiWriter(file, checksum), &contextReader{ctx: ctx, reader: body})
	if err != nil {
		return failure(err)
	}
	if err := ctx.Err(); err != nil {
		return failure(err)
	}
	if err := file.Sync(); err != nil {
		return failure(err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return failure(err)
	}
	var digest [32]byte
	copy(digest[:], checksum.Sum(nil))
	return file, Artifact{key: artifactKey(source, digest), size: size, sha256: digest}, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.reader.Read(p)
	if cancelErr := r.ctx.Err(); cancelErr != nil {
		return n, cancelErr
	}
	return n, err
}

type verifiedReader struct {
	ctx    context.Context
	body   io.ReadCloser
	want   Artifact
	digest hash.Hash
	size   int64
}

func checkedBody(ctx context.Context, body io.ReadCloser, want Artifact) io.ReadCloser {
	return &verifiedReader{ctx: ctx, body: body, want: want, digest: sha256.New()}
}

func (r *verifiedReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.body.Read(p)
	if n > 0 {
		r.size += int64(n)
		r.digest.Write(p[:n])
	}
	if cancelErr := r.ctx.Err(); cancelErr != nil {
		return n, cancelErr
	}
	if err == io.EOF && (r.size != r.want.size || !equalHash(r.digest.Sum(nil), r.want.sha256)) {
		return n, errors.New("tax artifact checksum or size mismatch")
	}
	return n, err
}

func (r *verifiedReader) Close() error { return r.body.Close() }

func equalHash(got []byte, want [32]byte) bool {
	return len(got) == len(want) && string(got) == string(want[:])
}
