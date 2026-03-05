//go:build !dev

package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// GetDistFS returns the embedded filesystem containing the frontend build output.
func GetDistFS() fs.FS {
	return distFS
}
