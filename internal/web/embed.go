package web

import (
	"embed"
	"io/fs"
)

//go:embed static/*
var staticFS embed.FS

// staticSubFS returns a sub-filesystem rooted at the "static" directory.
// Used by the server to serve files at /static/*.
func staticSubFS() fs.FS {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic("failed to create static sub-filesystem: " + err.Error())
	}
	return sub
}
