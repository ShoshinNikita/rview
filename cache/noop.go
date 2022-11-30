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
func (NoopCache) GetFilepath(rview.FileID) (string, error) { return "", ErrNoopCache }
func (NoopCache) Remove(rview.FileID) error                { return ErrNoopCache }

type NoopCleaner struct{}

func NewNoopCleaner() *NoopCleaner                 { return &NoopCleaner{} }
func (NoopCleaner) Shutdown(context.Context) error { return nil }
