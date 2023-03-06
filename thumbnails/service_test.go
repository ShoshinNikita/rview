package thumbnails

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"github.com/ShoshinNikita/rview/cache"
	"github.com/ShoshinNikita/rview/rview"
	"github.com/stretchr/testify/require"
)

func TestThumbnailService(t *testing.T) {
	t.Parallel()

	r := require.New(t)

	tempDir, err := os.MkdirTemp("", "rview-test-*")
	r.NoError(err)
	cache, err := cache.NewDiskCache(tempDir)
	r.NoError(err)

	service := NewThumbnailService(cache, 2)
	service.useOriginalImageThresholdSize = 10

	var resizedCount int
	service.resizeFn = func(originalFile, cacheFile string, id rview.FileID) error {
		resizedCount++
		return os.WriteFile(cacheFile, []byte("resized-content-"+id.GetName()), 0o600)
	}

	fileID := rview.NewFileID("1.jpg", time.Now().Unix())

	r.False(service.IsThumbnailReady(fileID))

	resizeStart := time.Now()

	err = service.SendTask(fileID, func(ctx context.Context, id rview.FileID) (io.ReadCloser, error) {
		time.Sleep(110 * time.Millisecond)
		return io.NopCloser(bytes.NewReader([]byte("original-content-" + id.String()))), nil
	})
	r.NoError(err)

	// Must take into account in-progress tasks.
	r.True(service.IsThumbnailReady(fileID))

	// Must ignore duplicate tasks.
	for i := 0; i < 3; i++ {
		err = service.SendTask(fileID, nil)
		r.NoError(err)
	}

	rc, err := service.OpenThumbnail(context.Background(), fileID)
	r.NoError(err)

	data, err := io.ReadAll(rc)
	r.NoError(err)
	r.Equal("resized-content-1.jpg", string(data))

	dur := time.Since(resizeStart)
	if dur < 200*time.Millisecond {
		t.Fatalf("image must be opened in >=200ms, got: %s", dur)
	}

	r.Equal(1, resizedCount)
	r.True(service.IsThumbnailReady(fileID))

	t.Run("remove resized file after error", func(t *testing.T) {
		r := require.New(t)

		fileID := rview.NewFileID("2.jpg", time.Now().Unix())

		service.resizeFn = func(originalFile, cacheFile string, id rview.FileID) error {
			// File must be created by vips, emulate it.
			f, err := os.Create(cacheFile)
			r.NoError(err)
			r.NoError(f.Close())

			r.NoError(service.cache.Check(fileID))

			return errors.New("some error")
		}

		err = service.SendTask(fileID, func(context.Context, rview.FileID) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader([]byte("long phrase to exceed threshold"))), nil
		})
		r.NoError(err)

		_, err = service.OpenThumbnail(context.Background(), fileID)
		r.ErrorIs(err, rview.ErrCacheMiss)

		// Cache file must be removed.
		r.ErrorIs(service.cache.Check(fileID), rview.ErrCacheMiss)
	})

	t.Run("use original file", func(t *testing.T) {
		r := require.New(t)

		fileID := rview.NewFileID("3.jpg", time.Now().Unix())

		var resizeCalled bool
		service.resizeFn = func(_, _ string, _ rview.FileID) error {
			resizeCalled = true
			return errors.New("should not be called")
		}

		err = service.SendTask(fileID, func(context.Context, rview.FileID) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader([]byte("x"))), nil
		})
		r.NoError(err)

		rc, err = service.OpenThumbnail(context.Background(), fileID)
		r.NoError(err)
		data, err := io.ReadAll(rc)
		r.NoError(err)
		r.Equal("x", string(data))
		r.False(resizeCalled)
	})

	r.NoError(service.Shutdown(context.Background()))
}

func TestThumbnailService_CanGenerateThumbnail(t *testing.T) {
	t.Parallel()

	r := require.New(t)

	now := time.Now().Unix()

	canGenerate := NewThumbnailService(nil, 0).CanGenerateThumbnail

	r.True(canGenerate(rview.NewFileID("/home/users/test.png", now)))
	r.True(canGenerate(rview.NewFileID("/home/users/test.pNg", now)))
	r.True(canGenerate(rview.NewFileID("/home/users/test.JPG", now)))
	r.True(canGenerate(rview.NewFileID("/home/users/test with space.jpeg", now)))
	r.True(canGenerate(rview.NewFileID("/test.gif", now)))
	r.False(canGenerate(rview.NewFileID("/home/users/x.txt", now)))
}
