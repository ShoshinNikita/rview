package web

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/ShoshinNikita/rview/cache"
	"github.com/ShoshinNikita/rview/config"
	"github.com/ShoshinNikita/rview/rview"
	"github.com/ShoshinNikita/rview/thumbnails"
	"github.com/stretchr/testify/require"
)

func TestServer_sendGenerateThumbnailTasks(t *testing.T) {
	t.Parallel()

	r := require.New(t)

	stub := newThumbnailServiceStub()
	s := NewServer(config.Config{}, nil, stub)

	zeroModTime := time.Unix(0, 0)

	gotInfo := s.sendGenerateThumbnailTasks(DirInfo{
		Entries: []DirEntry{
			{filepath: "a.txt", ModTime: zeroModTime},
			{filepath: "b.jpg", ModTime: zeroModTime},
			{filepath: "c.png", ModTime: zeroModTime},
			{filepath: "c.bmp", ModTime: zeroModTime},
			{filepath: "d.zip", ModTime: zeroModTime},
			{filepath: "error.jpg", ModTime: zeroModTime},
			{filepath: "resized.jpg", ModTime: zeroModTime},
		},
		dirURL: mustParseURL("/"),
	})
	r.Equal(3, stub.taskCount)

	r.Equal(
		[]DirEntry{
			{filepath: "a.txt", ModTime: zeroModTime}, // no thumbnail: text file
			{filepath: "b.jpg", ModTime: zeroModTime, ThumbnailURL: "/api/thumbnail/b.jpg?mod_time=0"},
			{filepath: "c.png", ModTime: zeroModTime, ThumbnailURL: "/api/thumbnail/c.png?mod_time=0"},
			{filepath: "c.bmp", ModTime: zeroModTime},     // no thumbnail: unsupported image
			{filepath: "d.zip", ModTime: zeroModTime},     // no thumbnail: archive
			{filepath: "error.jpg", ModTime: zeroModTime}, // no thumbnail: got error
			{filepath: "resized.jpg", ModTime: zeroModTime, ThumbnailURL: "/api/thumbnail/resized.jpg?mod_time=0"},
		},
		gotInfo.Entries,
	)
}

type thumbnailServiceStub struct {
	s rview.ThumbnailService

	taskCount int
}

func newThumbnailServiceStub() *thumbnailServiceStub {
	return &thumbnailServiceStub{
		s: thumbnails.NewThumbnailService(cache.NewNoopCache(), 0),
	}
}

func (s *thumbnailServiceStub) IsThumbnailReady(id rview.FileID) bool {
	return id.GetName() == "resized.jpg"
}

func (s *thumbnailServiceStub) SendTask(id rview.FileID, openFileFn rview.OpenFileFn) error {
	s.taskCount++

	if id.GetName() == "error.jpg" {
		return errors.New("error")
	}
	return nil
}

func (s *thumbnailServiceStub) CanGenerateThumbnail(id rview.FileID) bool {
	return s.s.CanGenerateThumbnail(id)
}

func (s *thumbnailServiceStub) OpenThumbnail(ctx context.Context, id rview.FileID) (io.ReadCloser, error) {
	return s.s.OpenThumbnail(ctx, id)
}

func (s *thumbnailServiceStub) Shutdown(ctx context.Context) error {
	return s.s.Shutdown(ctx)
}
