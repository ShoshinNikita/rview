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

func (NoopThumbnailService) StartThumbnailGeneration(rview.FileID, int64) (rview.ThumbnailID, error) {
	return rview.ThumbnailID{}, ErrNoopThumbnailService
}

func (NoopThumbnailService) Shutdown(context.Context) error {
	return nil
}

func (NoopThumbnailService) OpenThumbnail(context.Context, rview.ThumbnailID) (io.ReadCloser, error) {
	return nil, ErrNoopThumbnailService
}
