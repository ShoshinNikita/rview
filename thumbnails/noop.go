package thumbnails

import (
	"context"
	"errors"
	"io"

	"github.com/ShoshinNikita/rview/rview"
)

var ErrNoopThumbnailService = errors.New("noop thumbnail service")

type NoopThumbnailService struct{}

func NewNoopThumbnailService() *NoopThumbnailService {
	return &NoopThumbnailService{}
}

func (NoopThumbnailService) CanGenerateThumbnail(rview.FileID) bool {
	return false
}

func (NoopThumbnailService) OpenThumbnail(context.Context, rview.FileID, ThumbnailSize) (io.ReadCloser, string, error) {
	return nil, "", ErrNoopThumbnailService
}

func (NoopThumbnailService) Shutdown(context.Context) error {
	return nil
}
