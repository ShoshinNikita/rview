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

func (NoopThumbnailService) IsThumbnailReady(rview.FileID) bool {
	return false
}

func (NoopThumbnailService) SendTask(rview.FileID, rview.OpenFileFn) error {
	return ErrNoopThumbnailService
}

func (NoopThumbnailService) GetMimeType(rview.FileID) string {
	return ""
}

func (NoopThumbnailService) Shutdown(context.Context) error {
	return nil
}

func (NoopThumbnailService) OpenThumbnail(context.Context, rview.FileID) (io.ReadCloser, error) {
	return nil, ErrNoopThumbnailService
}
