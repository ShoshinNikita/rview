package static

import (
	"embed"
	"io/fs"
	"os"
	"path"
	"path/filepath"
)

func newFS(fs embed.FS, root string, debug bool) fs.FS {
	if debug {
		return dirFS(filepath.Join("static", root))
	}
	return &rootFS{
		internal: fs,
		root:     root,
	}
}

type dirFS string

func (root dirFS) Open(name string) (fs.File, error) {
	return os.Open(filepath.Join(string(root), name))
}

type rootFS struct {
	internal fs.FS
	root     string
}

func (fs *rootFS) Open(name string) (fs.File, error) {
	return fs.internal.Open(path.Join(fs.root, name))
}
