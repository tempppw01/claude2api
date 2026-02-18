package web

import (
	"embed"
	"io/fs"
)

//go:embed static
var StaticFS embed.FS

// GetStaticFS returns the embedded static files as a sub-filesystem
func GetStaticFS() (fs.FS, error) {
	return fs.Sub(StaticFS, "static")
}
