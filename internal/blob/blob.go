// Package blob is a content-addressed byte store on the local filesystem.
// Bytes live at <dir>/<hex[0:2]>/<hex>; Postgres keeps metadata/refcounts.
package blob

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type Store struct {
	Dir string
}

func New(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{Dir: dir}, nil
}

func (s *Store) pathFor(hash []byte) string {
	hx := hex.EncodeToString(hash)
	return filepath.Join(s.Dir, hx[:2], hx)
}

// Put stores the bytes and returns their sha256 hash. Idempotent.
func (s *Store) Put(data []byte) ([]byte, error) {
	sum := sha256.Sum256(data)
	p := s.pathFor(sum[:])
	if _, err := os.Stat(p); err == nil {
		return sum[:], nil
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return nil, err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp, p); err != nil {
		return nil, err
	}
	return sum[:], nil
}

func (s *Store) Get(hash []byte) ([]byte, error) {
	return os.ReadFile(s.pathFor(hash))
}

func (s *Store) Open(hash []byte) (io.ReadSeekCloser, error) {
	f, err := os.Open(s.pathFor(hash))
	if err != nil {
		return nil, fmt.Errorf("blob %x: %w", hash, err)
	}
	return f, nil
}

// Link hard-links (or copies) the blob to dst — used to materialize compile dirs.
func (s *Store) Link(hash []byte, dst string) error {
	src := s.pathFor(hash)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := os.Link(src, dst); err == nil {
		return nil
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}
