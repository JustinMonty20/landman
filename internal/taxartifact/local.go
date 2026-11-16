package taxartifact

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type localStore struct {
	root string
}

// NewLocal selects a private disk directory for single-instance runs.
func NewLocal(root string) (Store, error) {
	if root == "" {
		return nil, errors.New("local artifact directory is required")
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("local artifact directory must be private")
	}
	staging := filepath.Join(root, ".staging")
	if err := os.MkdirAll(staging, 0700); err != nil {
		return nil, err
	}
	if err := syncDirectoryChain(staging); err != nil {
		return nil, err
	}
	return &localStore{root: root}, nil
}

func (s *localStore) Mode() string { return "local" }

func (s *localStore) Put(ctx context.Context, source string, body io.Reader) (Artifact, error) {
	file, artifact, err := stage(ctx, filepath.Join(s.root, ".staging"), source, body)
	if err != nil {
		return Artifact{}, err
	}
	defer func() {
		file.Close()
		os.Remove(file.Name())
	}()
	path := filepath.Join(s.root, filepath.FromSlash(artifact.key))
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0700); err != nil {
		return Artifact{}, err
	}
	if err := os.Link(file.Name(), path); err != nil && !errors.Is(err, os.ErrExist) {
		return Artifact{}, fmt.Errorf("retain tax artifact: %w", err)
	}
	if err := Verify(ctx, s, artifact); err != nil {
		return Artifact{}, err
	}
	if err := syncDirectoryChain(parent); err != nil {
		return Artifact{}, err
	}
	artifact.mode = s.Mode()
	artifact.verified = true
	return artifact, nil
}

// syncDirectoryChain persists new directory entries, including entries left by a failed retry.
func syncDirectoryChain(path string) error {
	dir, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	for {
		file, err := os.Open(dir)
		if err != nil {
			return fmt.Errorf("open tax artifact directory %q: %w", dir, err)
		}
		syncErr := file.Sync()
		closeErr := file.Close()
		if err := errors.Join(syncErr, closeErr); err != nil {
			return fmt.Errorf("sync tax artifact directory %q: %w", dir, err)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil
		}
		dir = parent
	}
}

func (s *localStore) Get(ctx context.Context, artifact Artifact) (io.ReadCloser, error) {
	if err := validateArtifact(artifact); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	body, err := os.Open(filepath.Join(s.root, filepath.FromSlash(artifact.key)))
	if err != nil {
		return nil, err
	}
	return checkedBody(ctx, body, artifact), nil
}
