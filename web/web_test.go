package web

import (
	"testing"
	"time"

	"github.com/ShoshinNikita/rview/rclone"
	"github.com/ShoshinNikita/rview/rview"
	"github.com/ShoshinNikita/rview/thumbnails"
	"github.com/stretchr/testify/require"
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

	t.Run("thumbnails mode", func(t *testing.T) {
		r := require.New(t)

		thumbnailService := thumbnails.NewThumbnailService(nil, nil, nil, 0, rview.JpegThumbnails, true)
		s := NewServer(rview.Config{ImagePreviewMode: rview.ImagePreviewModeThumbnails}, nil, thumbnailService, nil)

		gotInfo := s.convertRcloneInfo(getTestRcloneInfo())
		resetUnnecessaryFields(&gotInfo)
		r.Equal(
			[]DirEntry{
				{
					Filename: "a.txt", FileType: rview.FileTypeText, CanPreview: true,
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

		s := NewServer(rview.Config{ImagePreviewMode: rview.ImagePreviewModeOriginal}, nil, nil, nil)

		gotInfo := s.convertRcloneInfo(getTestRcloneInfo())
		resetUnnecessaryFields(&gotInfo)
		r.Equal(
			[]DirEntry{
				{
					Filename: "a.txt", FileType: rview.FileTypeText, CanPreview: true,
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

		s := NewServer(rview.Config{ImagePreviewMode: rview.ImagePreviewModeNone}, nil, nil, nil)

		gotInfo := s.convertRcloneInfo(getTestRcloneInfo())
		resetUnnecessaryFields(&gotInfo)
		r.Equal(
			[]DirEntry{
				{Filename: "a.txt", FileType: rview.FileTypeText, CanPreview: true},
				{Filename: "b.jpg", FileType: rview.FileTypeImage},
				{Filename: "c.png", FileType: rview.FileTypeImage},
				{Filename: "c.bmp", FileType: rview.FileTypeImage},
				{Filename: "d.zip", FileType: rview.FileTypeUnknown},
			},
			gotInfo.Entries,
		)
	})
}
