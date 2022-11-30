package resizer

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
	"github.com/ShoshinNikita/rview/util/testutil"
)

func TestImageResizer(t *testing.T) {
	t.Parallel()

	tempDir, err := os.MkdirTemp("", "rview-test-*")
	testutil.NoError(t, err)
	cache, err := cache.NewDiskCache(tempDir)
	testutil.NoError(t, err)

	resizer := NewImageResizer(cache, 2)
	resizer.useOriginalImageThresholdSize = 10

	var resizedCount int
	resizer.resizeFn = func(originalFile, cacheFile string, id rview.FileID) error {
		resizedCount++
		return os.WriteFile(cacheFile, []byte("resized-content-"+id.GetName()), 0o600)
	}

	fileID := rview.NewFileID("1.jpg", time.Now().Unix())

	testutil.Equal(t, false, resizer.IsResized(fileID))

	resizeStart := time.Now()

	err = resizer.Resize(fileID, func(ctx context.Context, id rview.FileID) (io.ReadCloser, error) {
		time.Sleep(110 * time.Millisecond)
		return io.NopCloser(bytes.NewReader([]byte("original-content-" + id.String()))), nil
	})
	testutil.NoError(t, err)

	// Must take into account in-progress tasks.
	testutil.Equal(t, true, resizer.IsResized(fileID))

	// Must ignore duplicate tasks.
	for i := 0; i < 3; i++ {
		err = resizer.Resize(fileID, nil)
		testutil.NoError(t, err)
	}

	rc, err := resizer.OpenResized(context.Background(), fileID)
	testutil.NoError(t, err)

	data, err := io.ReadAll(rc)
	testutil.NoError(t, err)
	testutil.Equal(t, "resized-content-1.jpg", string(data))

	dur := time.Since(resizeStart)
	if dur < 200*time.Millisecond {
		t.Fatalf("image must be opened in >=200ms, got: %s", dur)
	}

	testutil.Equal(t, 1, resizedCount)
	testutil.Equal(t, true, resizer.IsResized(fileID))

	t.Run("remove resized file after error", func(t *testing.T) {
		fileID := rview.NewFileID("2.jpg", time.Now().Unix())

		resizer.resizeFn = func(originalFile, cacheFile string, id rview.FileID) error {
			// File must be created by vips, emulate it.
			f, err := os.Create(cacheFile)
			testutil.NoError(t, err)
			testutil.NoError(t, f.Close())

			testutil.NoError(t, resizer.cache.Check(fileID))

			return errors.New("some error")
		}

		err = resizer.Resize(fileID, func(context.Context, rview.FileID) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader([]byte("long phrase to exceed threshold"))), nil
		})
		testutil.NoError(t, err)

		_, err = resizer.OpenResized(context.Background(), fileID)
		testutil.IsError(t, err, rview.ErrCacheMiss)

		// Cache file must be removed.
		testutil.IsError(t, resizer.cache.Check(fileID), rview.ErrCacheMiss)
	})

	t.Run("use original file", func(t *testing.T) {
		fileID := rview.NewFileID("3.jpg", time.Now().Unix())

		var resizeCalled bool
		resizer.resizeFn = func(_, _ string, _ rview.FileID) error {
			resizeCalled = true
			return errors.New("should not be called")
		}

		err = resizer.Resize(fileID, func(context.Context, rview.FileID) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader([]byte("x"))), nil
		})
		testutil.NoError(t, err)

		rc, err = resizer.OpenResized(context.Background(), fileID)
		testutil.NoError(t, err)
		data, err := io.ReadAll(rc)
		testutil.NoError(t, err)
		testutil.Equal(t, "x", string(data))
		testutil.Equal(t, false, resizeCalled)
	})

	testutil.NoError(t, resizer.Shutdown(context.Background()))
}

func TestImageResizer_CanResize(t *testing.T) {
	t.Parallel()

	now := time.Now().Unix()

	resizer := NewImageResizer(nil, 0)

	testutil.Equal(t, true, resizer.CanResize(rview.NewFileID("/home/users/test.png", now)))
	testutil.Equal(t, true, resizer.CanResize(rview.NewFileID("/home/users/test.pNg", now)))
	testutil.Equal(t, true, resizer.CanResize(rview.NewFileID("/home/users/test.JPG", now)))
	testutil.Equal(t, true, resizer.CanResize(rview.NewFileID("/home/users/test with space.jpeg", now)))
	testutil.Equal(t, false, resizer.CanResize(rview.NewFileID("/home/users/x.txt", now)))
}
