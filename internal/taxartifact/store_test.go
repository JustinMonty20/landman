package taxartifact

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLocalContract(t *testing.T) {
	store, err := NewLocal(filepath.Join(t.TempDir(), "private"))
	if err != nil {
		t.Fatal(err)
	}
	testContract(t, store)
}

func testContract(t *testing.T, store Store) {
	t.Helper()
	ctx := context.Background()
	one := []byte("exact source bytes\x00\xff\r\n")
	a, err := store.Put(ctx, "union_delinquent", bytes.NewReader(one))
	if err != nil {
		t.Fatal(err)
	}
	if !a.Verified() || a.Mode() != store.Mode() || a.Key() != artifactKey("union_delinquent", sha256.Sum256(one)) || a.Size() != int64(len(one)) || a.SHA256() != sha256.Sum256(one) {
		t.Fatalf("wrong artifact reference: %+v", a)
	}
	read, err := store.Get(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(read)
	if err != nil {
		t.Fatal(err)
	}
	if err := read.Close(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, one) {
		t.Fatalf("read %q, want exact bytes %q", got, one)
	}
	if err := Verify(ctx, store, a); err != nil {
		t.Fatal(err)
	}
	untrusted, err := Reference(a.Key(), a.Size(), a.SHA256())
	if err != nil || untrusted.Verified() {
		t.Fatalf("reference unexpectedly trusted: %v", err)
	}
	trusted, err := VerifiedReference(ctx, store, untrusted)
	if err != nil || trusted != a {
		t.Fatalf("verified reference differs from retained artifact: %+v %v", trusted, err)
	}
	if _, err := VerifiedReference(ctx, store, Artifact{key: a.Key(), size: a.Size() + 1, sha256: a.SHA256()}); err == nil {
		t.Fatal("accepted mismatched retained artifact size")
	}
	retry, err := store.Put(ctx, "union_delinquent", bytes.NewReader(one))
	if err != nil || retry != a {
		t.Fatalf("identical retry: %+v %v", retry, err)
	}
	b, err := store.Put(ctx, "union_delinquent", strings.NewReader("different version"))
	if err != nil || b.Key() == a.Key() {
		t.Fatalf("different version: %+v %v", b, err)
	}
	for _, version := range []Artifact{a, b} {
		if err := Verify(ctx, store, version); err != nil {
			t.Fatalf("retained version %s: %v", version.Key(), err)
		}
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := store.Put(cancelled, "union_delinquent", strings.NewReader("unpublished")); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled put: %v", err)
	}
	if err := Verify(ctx, store, a); err != nil {
		t.Fatalf("cancel changed prior version: %v", err)
	}
	midReadCtx, cancelMidRead := context.WithCancel(ctx)
	if _, err := store.Put(midReadCtx, "union_delinquent", &cancellingReader{cancel: cancelMidRead}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel during staging: %v", err)
	}
	if err := Verify(ctx, store, a); err != nil {
		t.Fatalf("staging cancellation changed prior version: %v", err)
	}
	readCtx, cancelRead := context.WithCancel(ctx)
	body, err := store.Get(readCtx, a)
	if err != nil {
		t.Fatal(err)
	}
	var firstByte [1]byte
	if n, err := body.Read(firstByte[:]); n != 1 || err != nil {
		body.Close()
		t.Fatalf("read before cancellation: n=%d err=%v", n, err)
	}
	cancelRead()
	if _, err := body.Read(firstByte[:]); !errors.Is(err, context.Canceled) {
		body.Close()
		t.Fatalf("read after cancellation: %v", err)
	}
	if err := body.Close(); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("source read failed")
	if _, err := store.Put(ctx, "union_delinquent", &failingReader{err: failure}); !errors.Is(err, failure) {
		t.Fatalf("failed put: %v", err)
	}
	if err := Verify(ctx, store, a); err != nil {
		t.Fatalf("source failure changed prior version: %v", err)
	}
	wrong, err := Reference(a.Key(), a.Size()+1, a.SHA256())
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(ctx, store, wrong); err == nil {
		t.Fatal("expected length verification error")
	}
	if _, err := Reference(a.Key(), a.Size(), sha256.Sum256([]byte("other"))); err == nil {
		t.Fatal("expected invalid reference error")
	}
	if _, err := store.Put(ctx, "../bad", strings.NewReader("x")); err == nil {
		t.Fatal("accepted unsafe source")
	}
}

type cancellingReader struct {
	cancel context.CancelFunc
}

func (r *cancellingReader) Read(p []byte) (int, error) {
	r.cancel()
	return copy(p, "partial"), nil
}

type failingReader struct {
	err  error
	read bool
}

func (r *failingReader) Read(p []byte) (int, error) {
	if !r.read {
		r.read = true
		return copy(p, "partial"), nil
	}
	return 0, r.err
}

func TestLocalRejectsCorruptRetry(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private")
	store, err := NewLocal(root)
	if err != nil {
		t.Fatal(err)
	}
	a, err := store.Put(context.Background(), "union_billing", strings.NewReader("original"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, filepath.FromSlash(a.Key()))
	if err := os.WriteFile(path, []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Put(context.Background(), "union_billing", strings.NewReader("original")); err == nil {
		t.Fatal("corrupt existing object was accepted")
	}
	body, err := os.ReadFile(path)
	if err != nil || string(body) != "tampered" {
		t.Fatalf("corrupt object was overwritten: %q, %v", body, err)
	}
}

func TestLocalLargeStreamHasBoundedHeap(t *testing.T) {
	store, err := NewLocal(filepath.Join(t.TempDir(), "private"))
	if err != nil {
		t.Fatal(err)
	}
	const size = 32 << 20
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	artifact, err := store.Put(context.Background(), "union_billing", io.LimitReader(zeroReader{}, size))
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Size() != size || after.TotalAlloc-before.TotalAlloc > 8<<20 {
		t.Fatalf("size=%d, Go allocations=%d for %d-byte stream", artifact.Size(), after.TotalAlloc-before.TotalAlloc, size)
	}
	if err := Verify(context.Background(), store, artifact); err != nil {
		t.Fatal(err)
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

func TestLocalRequiresPrivateRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := NewLocal(root); err == nil {
		t.Fatal("accepted public artifact directory")
	}
}
