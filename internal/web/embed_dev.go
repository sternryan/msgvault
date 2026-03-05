//go:build dev

package web

import (
	"io/fs"
	"os"
)

// GetDistFS returns the filesystem for dev mode, serving from disk.
// Falls back to an empty in-memory FS if the dist directory doesn't exist.
func GetDistFS() fs.FS {
	if info, err := os.Stat("internal/web/dist"); err == nil && info.IsDir() {
		return os.DirFS("internal/web")
	}
	if info, err := os.Stat("web/dist"); err == nil && info.IsDir() {
		return os.DirFS("web")
	}
	// Return a minimal FS with a placeholder
	return emptyFS{}
}

type emptyFS struct{}

func (emptyFS) Open(name string) (fs.File, error) {
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}
