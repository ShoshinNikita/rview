package rview

import (
	"errors"
	"io"
	"path"
	"time"
)

type FileID struct {
	path    string // full path
	name    string // only filename
	modTime int64  // unix time
}

// NewFileID returns a new [FileID] with cleaned filepath and filename.
func NewFileID(filepath string, modTime int64) FileID {
	filepath = path.Clean(filepath)
	name := path.Base(filepath)

	return FileID{
		path:    filepath,
		name:    name,
		modTime: modTime,
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

// GetModTime returns the modification time.
func (id FileID) GetModTime() time.Time {
	return time.Unix(id.modTime, 0)
}

var (
	ErrCacheMiss = errors.New("cache miss")
)

type Cache interface {
	Open(id FileID) (io.ReadCloser, error)
	Check(id FileID) error
	GetSaveWriter(id FileID) (io.WriteCloser, error)
}
