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

	thumbnailService := thumbnails.NewThumbnailService(cache, 1, true)

	for _, tt := range []struct {
		imageType string
		file      string
		mimeType  string
	}{
		{imageType: "jpg", file: "Images/birds-g64b44607c_640.jpg", mimeType: "image/jpeg"},
		{imageType: "png", file: "Images/ytrewq.png", mimeType: "image/png"},
		{imageType: "webp", file: "Images/qwerty.webp", mimeType: "image/webp"},
		{imageType: "heic", file: "Images/asdfgh.heic", mimeType: "image/jpeg"}, // we should generate .jpeg thumbnails for .heic images
	} {
		tt := tt
		t.Run(tt.imageType, func(t *testing.T) {
			r := require.New(t)

			fileID := rview.NewFileID(tt.file, 0)

			originalImage, err := os.ReadFile(filepath.Join("testdata", tt.file))
			r.NoError(err)

			err = thumbnailService.SendTask(fileID, func(context.Context, rview.FileID) (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(originalImage)), nil
			})
			r.NoError(err)

			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			rc, err := thumbnailService.OpenThumbnail(ctx, fileID)
			r.NoError(err)
			defer rc.Close()

			thumbnail, err := io.ReadAll(rc)
			r.NoError(err)
			r.NotEqual(len(thumbnail), len(originalImage), "size of thumbnail and original file should differ")

			mimeType := thumbnailService.GetMimeType(fileID)
			r.Equal(tt.mimeType, mimeType)
		})
	}
}
