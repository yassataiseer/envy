//go:build !windows

package runner

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestLookPath(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "tool")
	noexec := filepath.Join(dir, "data")
	os.WriteFile(exe, []byte("#!/bin/sh\n"), 0o755)
	os.WriteFile(noexec, []byte("x"), 0o644)

	if p, err := lookPath("tool", "/nonexistent:"+dir); err != nil || p != exe {
		t.Errorf("lookPath(tool) = %q, %v", p, err)
	}
	if _, err := lookPath("missing", dir); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing: %v", err)
	}
	if _, err := lookPath(noexec, ""); !errors.Is(err, fs.ErrPermission) {
		t.Errorf("non-executable: %v", err)
	}
	if _, err := lookPath(dir, ""); !errors.Is(err, fs.ErrPermission) {
		t.Errorf("directory: %v", err)
	}
	if p, err := lookPath(exe, ""); err != nil || p != exe {
		t.Errorf("absolute path: %q %v", p, err)
	}
}
