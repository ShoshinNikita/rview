package ui

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed *
var templates embed.FS

func New(debug bool) fs.FS {
	if debug {
		return dirFS("ui")
	}
	return templates
}

type dirFS string

func (root dirFS) Open(name string) (fs.File, error) {
	return os.Open(filepath.Join(string(root), name))
}
