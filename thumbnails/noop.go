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

func (NoopThumbnailService) NewThumbnailID(id rview.FileID) rview.ThumbnailID {
	return rview.ThumbnailID{FileID: id}
}

func (NoopThumbnailService) CanGenerateThumbnail(rview.FileID) bool {
	return false
}

func (NoopThumbnailService) IsThumbnailReady(rview.ThumbnailID) bool {
	return false
}

func (NoopThumbnailService) SendTask(rview.FileID) error {
	return ErrNoopThumbnailService
}

func (NoopThumbnailService) Shutdown(context.Context) error {
	return nil
}

func (NoopThumbnailService) OpenThumbnail(context.Context, rview.ThumbnailID) (io.ReadCloser, error) {
	return nil, ErrNoopThumbnailService
}
