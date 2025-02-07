package tests

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ShoshinNikita/rview/pkg/cache"
	"github.com/ShoshinNikita/rview/rview"
	"github.com/ShoshinNikita/rview/thumbnails"
	"github.com/stretchr/testify/require"
)

// TestThumbnailGeneration checks that we can successfully generate
// thumbnails for all supported image types.
func TestThumbnailGeneration(t *testing.T) {
	cache, err := cache.NewDiskCache(t.TempDir())
	require.NoError(t, err)

	type Test struct {
		imageType string
		file      string
		mimeType  string
		sameSize  bool
	}
	runTests := func(t *testing.T, format rview.ThumbnailsFormat, tests []Test) {
		for _, tt := range tests {
			t.Run(tt.imageType, func(t *testing.T) {
				r := require.New(t)

				originalImage, err := os.ReadFile(filepath.Join("testdata", tt.file))
				r.NoError(err)

				openFileFn := func(context.Context, rview.FileID) (io.ReadCloser, error) {
					return io.NopCloser(bytes.NewReader(originalImage)), nil
				}
				thumbnailService := thumbnails.NewThumbnailService(openFileFn, cache, 1, format)
				thumbnailService.GenerateThumbnailsForSmallFiles()

				fileID := rview.NewFileID(tt.file, 0, 0)

				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				rc, contentType, err := thumbnailService.OpenThumbnail(ctx, fileID)
				r.NoError(err)
				defer rc.Close()

				thumbnail, err := io.ReadAll(rc)
				r.NoError(err)
				if tt.sameSize {
					r.Equal(len(thumbnail), len(originalImage), "size of thumbnail and original file should be equal")
				} else {
					r.NotEqual(len(thumbnail), len(originalImage), "size of thumbnail and original file should differ")
				}

				r.Equal(tt.mimeType, contentType)
			})
		}
	}

	t.Run("jpeg", func(t *testing.T) {
		runTests(t, rview.JpegThumbnails, []Test{
			{imageType: "jpg", file: "Images/birds-g64b44607c_640.jpg", mimeType: "image/jpeg"},
			{imageType: "png", file: "Images/ytrewq.png", mimeType: "image/jpeg"},
			{imageType: "webp", file: "Images/qwerty.webp", mimeType: "image/webp"},
			{imageType: "heic", file: "Images/asdfgh.heic", mimeType: "image/jpeg"}, // we should generate .jpeg thumbnails for .heic images
			{imageType: "avif", file: "Images/sky.avif", mimeType: "image/avif"},
			{imageType: "gif", file: "test.gif", mimeType: "image/gif", sameSize: true}, // we save the original file
		})
	})

	t.Run("avif", func(t *testing.T) {
		runTests(t, rview.AvifThumbnails, []Test{
			{imageType: "jpg", file: "Images/birds-g64b44607c_640.jpg", mimeType: "image/avif"},
			{imageType: "png", file: "Images/ytrewq.png", mimeType: "image/avif"},
			{imageType: "webp", file: "Images/qwerty.webp", mimeType: "image/webp"},
			{imageType: "heic", file: "Images/asdfgh.heic", mimeType: "image/avif"}, // we should generate .avif thumbnails for .heic images
			{imageType: "avif", file: "Images/sky.avif", mimeType: "image/avif"},
			{imageType: "gif", file: "test.gif", mimeType: "image/gif", sameSize: true}, // we save the original file
		})
	})
}
