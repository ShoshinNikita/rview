package rview

import (
	"fmt"
	"path"
	"strings"
)

type FileID struct {
	path    string // full path
	name    string // only filename
	modTime int64  // unix time
	size    int64
}

// NewFileID returns a new [FileID] with cleaned filepath and filename.
func NewFileID(filepath string, modTime int64, size int64) FileID {
	filepath = path.Clean(filepath)
	name := path.Base(filepath)

	return FileID{
		path:    filepath,
		name:    name,
		modTime: modTime,
		size:    size,
	}
}

// GetPath returns full filepath.
func (id FileID) GetPath() string {
	return id.path
}

// GetName returns only filename (last path element).
func (id FileID) GetName() string {
	return id.name
}

// GetExt returns the filename extension in lower case with leading dot (.html).
func (id FileID) GetExt() string {
	return GetFileExt(id.name)
}

func GetFileExt(filepath string) string {
	return strings.ToLower(path.Ext(filepath))
}

// GetModTime returns the modification time.
func (id FileID) GetModTime() int64 {
	return id.modTime
}

// GetSize returns the file size
func (id FileID) GetSize() int64 {
	return id.size
}

func (id FileID) String() string {
	return fmt.Sprintf("t%d_s%d_%s", id.modTime, id.size, id.path)
}
