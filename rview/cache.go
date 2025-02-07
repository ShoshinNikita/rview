package rview

import (
	"context"
	"errors"
	"io"
)

var (
	ErrCacheMiss = errors.New("cache miss")
)

type Cache interface {
	Open(id FileID) (io.ReadCloser, error)
	Check(id FileID) error
	GetFilepath(id FileID) (path string, err error)
	Write(id FileID, r io.Reader) (err error)
	Remove(id FileID) error
}

type CacheCleaner interface {
	Shutdown(ctx context.Context) error
}
