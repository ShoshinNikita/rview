package web

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/ShoshinNikita/rview/pkg/cache"
	"github.com/ShoshinNikita/rview/rview"
	"github.com/ShoshinNikita/rview/thumbnails"
	"github.com/stretchr/testify/require"
)

func TestServer_convertRcloneInfo(t *testing.T) {
	t.Parallel()

	getTestRcloneInfo := func() *rview.RcloneDirInfo {
		return &rview.RcloneDirInfo{
			Entries: []rview.RcloneDirEntry{
				{URL: "a.txt"},
				{URL: "b.jpg"},
				{URL: "c.png"},
				{URL: "c.bmp"},
				{URL: "d.zip"},
				{URL: "error.jpg"},
				{URL: "resized.jpg"},
			},
		}
	}
	resetUnnecessaryFields := func(info *DirInfo) {
		for i := range info.Entries {
			info.Entries[i].filepath = ""
			info.Entries[i].HumanReadableSize = ""
			info.Entries[i].ModTime = time.Time{}
			info.Entries[i].HumanReadableModTime = ""
			info.Entries[i].OriginalFileURL = ""
			info.Entries[i].IconName = ""
		}
	}

	t.Run("thumbnails mode", func(t *testing.T) {
		r := require.New(t)

		stub := newThumbnailServiceStub()
		s := NewServer(rview.Config{ImagePreviewMode: rview.ImagePreviewModeThumbnails}, nil, stub, nil)

		gotInfo, err := s.convertRcloneInfo(getTestRcloneInfo(), "/")
		r.NoError(err)
		r.Equal(3, stub.taskCount)
		resetUnnecessaryFields(&gotInfo)
		r.Equal(
			[]DirEntry{
				{
					Filename: "a.txt", FileType: rview.FileTypeText, CanPreview: true,
					ThumbnailURL: "", // no thumbnail: text file
				},
				{
					Filename: "b.jpg", FileType: rview.FileTypeImage,
					ThumbnailURL: "/api/thumbnail/b.thumbnail.jpg?mod_time=0", CanPreview: true,
				},
				{
					Filename: "c.png", FileType: rview.FileTypeImage,
					ThumbnailURL: "/api/thumbnail/c.thumbnail.png.jpeg?mod_time=0", CanPreview: true,
				},
				{
					Filename: "c.bmp", FileType: rview.FileTypeImage,
					ThumbnailURL: "", // no thumbnail: unsupported image
				},
				{
					Filename: "d.zip", FileType: rview.FileTypeUnknown,
					ThumbnailURL: "", // no thumbnail: archive
				},
				{
					Filename: "error.jpg", FileType: rview.FileTypeImage,
					ThumbnailURL: "", // no thumbnail: got error
				},
				{
					Filename: "resized.jpg", FileType: rview.FileTypeImage,
					ThumbnailURL: "/api/thumbnail/resized.thumbnail.jpg?mod_time=0", CanPreview: true,
				},
			},
			gotInfo.Entries,
		)
	})

	t.Run("original mode", func(t *testing.T) {
		r := require.New(t)

		s := NewServer(rview.Config{ImagePreviewMode: rview.ImagePreviewModeOriginal}, nil, nil, nil)

		gotInfo, err := s.convertRcloneInfo(getTestRcloneInfo(), "/")
		r.NoError(err)
		resetUnnecessaryFields(&gotInfo)
		r.Equal(
			[]DirEntry{
				{
					Filename: "a.txt", FileType: rview.FileTypeText, CanPreview: true,
				},
				{
					Filename: "b.jpg", FileType: rview.FileTypeImage,
					ThumbnailURL: "/api/file/b.jpg?mod_time=0", CanPreview: true,
				},
				{
					Filename: "c.png", FileType: rview.FileTypeImage,
					ThumbnailURL: "/api/file/c.png?mod_time=0", CanPreview: true,
				},
				{
					Filename: "c.bmp", FileType: rview.FileTypeImage,
					ThumbnailURL: "/api/file/c.bmp?mod_time=0", CanPreview: true,
				},
				{
					Filename: "d.zip", FileType: rview.FileTypeUnknown,
				},
				{
					Filename: "error.jpg", FileType: rview.FileTypeImage,
					ThumbnailURL: "/api/file/error.jpg?mod_time=0", CanPreview: true,
				},
				{
					Filename: "resized.jpg", FileType: rview.FileTypeImage,
					ThumbnailURL: "/api/file/resized.jpg?mod_time=0", CanPreview: true,
				},
			},
			gotInfo.Entries,
		)
	})

	t.Run("no preview mode", func(t *testing.T) {
		r := require.New(t)

		s := NewServer(rview.Config{ImagePreviewMode: rview.ImagePreviewModeNone}, nil, nil, nil)

		gotInfo, err := s.convertRcloneInfo(getTestRcloneInfo(), "/")
		r.NoError(err)
		resetUnnecessaryFields(&gotInfo)
		r.Equal(
			[]DirEntry{
				{Filename: "a.txt", FileType: rview.FileTypeText, CanPreview: true},
				{Filename: "b.jpg", FileType: rview.FileTypeImage},
				{Filename: "c.png", FileType: rview.FileTypeImage},
				{Filename: "c.bmp", FileType: rview.FileTypeImage},
				{Filename: "d.zip", FileType: rview.FileTypeUnknown},
				{Filename: "error.jpg", FileType: rview.FileTypeImage},
				{Filename: "resized.jpg", FileType: rview.FileTypeImage},
			},
			gotInfo.Entries,
		)
	})
}

type thumbnailServiceStub struct {
	s rview.ThumbnailService

	taskCount int
}

func newThumbnailServiceStub() *thumbnailServiceStub {
	return &thumbnailServiceStub{
		s: thumbnails.NewThumbnailService(nil, cache.NewInMemoryCache(), 0, rview.JpegThumbnails, false),
	}
}

func (s *thumbnailServiceStub) NewThumbnailID(id rview.FileID) rview.ThumbnailID {
	return s.s.NewThumbnailID(id)
}

func (s *thumbnailServiceStub) IsThumbnailReady(id rview.ThumbnailID) bool {
	return id.GetName() == "resized.thumbnail.jpg"
}

func (s *thumbnailServiceStub) SendTask(id rview.FileID) error {
	s.taskCount++

	if id.GetName() == "error.jpg" {
		return errors.New("error")
	}
	return nil
}

func (s *thumbnailServiceStub) CanGenerateThumbnail(id rview.FileID) bool {
	return s.s.CanGenerateThumbnail(id)
}

func (s *thumbnailServiceStub) OpenThumbnail(ctx context.Context, id rview.ThumbnailID) (io.ReadCloser, error) {
	return s.s.OpenThumbnail(ctx, id)
}

func (s *thumbnailServiceStub) Shutdown(ctx context.Context) error {
	return s.s.Shutdown(ctx)
}
