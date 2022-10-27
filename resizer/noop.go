package resizer

import (
	"context"
	"errors"
	"io"

	"github.com/ShoshinNikita/rview/rview"
)

var ErrNoopImageResizer = errors.New("noop image resizer")

type NoopImageResizer struct{}

func NewNoopImageResizer() *NoopImageResizer                         { return &NoopImageResizer{} }
func (NoopImageResizer) CanResize(rview.FileID) bool                 { return false }
func (NoopImageResizer) IsResized(rview.FileID) bool                 { return false }
func (NoopImageResizer) Resize(rview.FileID, rview.OpenFileFn) error { return ErrNoopImageResizer }
func (NoopImageResizer) Shutdown(context.Context) error              { return nil }
func (NoopImageResizer) OpenResized(context.Context, rview.FileID) (io.ReadCloser, error) {
	return nil, ErrNoopImageResizer
}
