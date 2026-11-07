package web

import (
	"context"
	"testing"
	"time"

	"github.com/ShoshinNikita/rview/index"
	"github.com/ShoshinNikita/rview/pkg/require"
	"github.com/ShoshinNikita/rview/rclone"
	"github.com/ShoshinNikita/rview/rview"
	"github.com/ShoshinNikita/rview/thumbnails"
)

func TestServer_convertRcloneInfo(t *testing.T) {
	t.Parallel()

	getTestRcloneInfo := func() *rclone.DirInfo {
		return &rclone.DirInfo{
			Dir: "/",
			Entries: []rclone.DirEntry{
				{URL: "/a.txt", Leaf: "a.txt"},
				{URL: "/b.jpg", Leaf: "b.jpg"},
				{URL: "/c.png", Leaf: "c.png"},
				{URL: "/c.bmp", Leaf: "c.bmp"},
				{URL: "/d.zip", Leaf: "d.zip"},
			},
		}
	}
	resetUnnecessaryFields := func(info *DirInfo) {
		for i := range info.Entries {
			info.Entries[i].HumanReadableSize = ""
			info.Entries[i].ModTime = time.Time{}
			info.Entries[i].HumanReadableModTime = ""
			info.Entries[i].OriginalFileURL = ""
			info.Entries[i].IconName = ""
		}
	}

	indexService := &indexServiceMock{
		GetAnnotationsFn: func(id rview.FileID) []string {
			if id.GetPath() == "/a.txt" {
				return []string{"text"}
			}
			return nil
		},
	}

	t.Run("thumbnails mode", func(t *testing.T) {
		r := require.New(t)

		thumbnailService := thumbnails.NewThumbnailService(nil, nil, nil, 0, rview.JpegThumbnails, true)
		s := NewServer(rview.Config{ImagePreviewMode: rview.ImagePreviewModeThumbnails}, nil, thumbnailService, indexService)

		gotInfo := s.convertRcloneInfo(getTestRcloneInfo())
		resetUnnecessaryFields(&gotInfo)
		r.Equal(
			[]DirEntry{
				{
					Filename: "a.txt", FileType: rview.FileTypeText, CanPreview: true,
					Annotations:  []string{"text"},
					ThumbnailURL: "", // no thumbnail: text file
				},
				{
					Filename: "b.jpg", FileType: rview.FileTypeImage,
					ThumbnailURL: "/api/thumbnail/b.jpg?mod_time=0&size=0", CanPreview: true,
				},
				{
					Filename: "c.png", FileType: rview.FileTypeImage,
					ThumbnailURL: "/api/thumbnail/c.png?mod_time=0&size=0", CanPreview: true,
				},
				{
					Filename: "c.bmp", FileType: rview.FileTypeImage,
					ThumbnailURL: "", // no thumbnail: unsupported image
				},
				{
					Filename: "d.zip", FileType: rview.FileTypeUnknown,
					ThumbnailURL: "", // no thumbnail: archive
				},
			},
			gotInfo.Entries,
		)
	})

	t.Run("original mode", func(t *testing.T) {
		r := require.New(t)

		s := NewServer(rview.Config{ImagePreviewMode: rview.ImagePreviewModeOriginal}, nil, nil, indexService)

		gotInfo := s.convertRcloneInfo(getTestRcloneInfo())
		resetUnnecessaryFields(&gotInfo)
		r.Equal(
			[]DirEntry{
				{
					Filename: "a.txt", FileType: rview.FileTypeText, CanPreview: true,
					Annotations: []string{"text"},
				},
				{
					Filename: "b.jpg", FileType: rview.FileTypeImage,
					ThumbnailURL: "/api/file/b.jpg?mod_time=0&size=0", CanPreview: true,
				},
				{
					Filename: "c.png", FileType: rview.FileTypeImage,
					ThumbnailURL: "/api/file/c.png?mod_time=0&size=0", CanPreview: true,
				},
				{
					Filename: "c.bmp", FileType: rview.FileTypeImage,
					ThumbnailURL: "/api/file/c.bmp?mod_time=0&size=0", CanPreview: true,
				},
				{
					Filename: "d.zip", FileType: rview.FileTypeUnknown,
				},
			},
			gotInfo.Entries,
		)
	})

	t.Run("no preview mode", func(t *testing.T) {
		r := require.New(t)

		s := NewServer(rview.Config{ImagePreviewMode: rview.ImagePreviewModeNone}, nil, nil, indexService)

		gotInfo := s.convertRcloneInfo(getTestRcloneInfo())
		resetUnnecessaryFields(&gotInfo)
		r.Equal(
			[]DirEntry{
				{Filename: "a.txt", FileType: rview.FileTypeText, CanPreview: true, Annotations: []string{"text"}},
				{Filename: "b.jpg", FileType: rview.FileTypeImage},
				{Filename: "c.png", FileType: rview.FileTypeImage},
				{Filename: "c.bmp", FileType: rview.FileTypeImage},
				{Filename: "d.zip", FileType: rview.FileTypeUnknown},
			},
			gotInfo.Entries,
		)
	})
}

type indexServiceMock struct {
	GetAnnotationsFn     func(rview.FileID) []string
	GetDefaultSortFn     func(dir string) (sort string, order string, ok bool)
	GetMinSearchLengthFn func() int
	RefreshIndexFn       func(context.Context) (finalErr error)
	SearchFn             func(ctx context.Context, search string, limit int) (hits []index.Hit, total int, err error)
}

func (i *indexServiceMock) GetAnnotations(id rview.FileID) []string {
	return i.GetAnnotationsFn(id)
}

func (i *indexServiceMock) GetDefaultSort(dir string) (sort string, order string, ok bool) {
	return i.GetDefaultSortFn(dir)
}

func (i *indexServiceMock) GetMinSearchLength() int {
	return i.GetMinSearchLengthFn()
}

func (i *indexServiceMock) RefreshIndex(ctx context.Context) (finalErr error) {
	return i.RefreshIndexFn(ctx)
}

func (i *indexServiceMock) Search(ctx context.Context, search string, limit int) (hits []index.Hit, total int, err error) {
	return i.SearchFn(ctx, search, limit)
}
