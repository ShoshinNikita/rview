package cache

import (
	"context"
	"errors"
	"io"

	"github.com/ShoshinNikita/rview/rview"
)

var ErrNoopCache = errors.New("noop cache")

type NoopCache struct{}

func NewNoopCache() *NoopCache                             { return &NoopCache{} }
func (NoopCache) Open(rview.FileID) (io.ReadCloser, error) { return nil, ErrNoopCache }
func (NoopCache) Check(rview.FileID) error                 { return ErrNoopCache }
func (NoopCache) GetSaveWriter(rview.FileID) (io.WriteCloser, func(), error) {
	return nopWriteCloser{io.Discard}, func() {}, nil
}

type nopWriteCloser struct {
	io.Writer
}

func (nopWriteCloser) Close() error {
	return nil
}

type NoopCleaner struct{}

func NewNoopCleaner() *NoopCleaner                 { return &NoopCleaner{} }
func (NoopCleaner) Shutdown(context.Context) error { return nil }
