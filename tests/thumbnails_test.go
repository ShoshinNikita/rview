package tests

import (
	"bytes"
	"context"
	"io"
	"mime"
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

	for _, tt := range []struct {
		imageType string
		file      string
		mimeType  string
	}{
		{imageType: "jpg", file: "Images/birds-g64b44607c_640.jpg", mimeType: "image/jpeg"},
		{imageType: "png", file: "Images/ytrewq.png", mimeType: "image/jpeg"},
		{imageType: "webp", file: "Images/qwerty.webp", mimeType: "image/webp"},
		{imageType: "heic", file: "Images/asdfgh.heic", mimeType: "image/jpeg"}, // we should generate .jpeg thumbnails for .heic images
	} {
		tt := tt
		t.Run(tt.imageType, func(t *testing.T) {
			r := require.New(t)

			originalImage, err := os.ReadFile(filepath.Join("testdata", tt.file))
			r.NoError(err)

			openFileFn := func(context.Context, rview.FileID) (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(originalImage)), nil
			}
			thumbnailService := thumbnails.NewThumbnailService(openFileFn, cache, 1, true)

			fileID := rview.NewFileID(tt.file, 0)
			thumbnailID := thumbnailService.NewThumbnailID(fileID)

			err = thumbnailService.SendTask(fileID)
			r.NoError(err)

			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			rc, err := thumbnailService.OpenThumbnail(ctx, thumbnailID)
			r.NoError(err)
			defer rc.Close()

			thumbnail, err := io.ReadAll(rc)
			r.NoError(err)
			r.NotEqual(len(thumbnail), len(originalImage), "size of thumbnail and original file should differ")

			mimeType := mime.TypeByExtension(thumbnailID.GetExt())
			r.Equal(tt.mimeType, mimeType)
		})
	}
}
