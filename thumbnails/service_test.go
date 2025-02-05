package thumbnails

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"testing"
	"time"

	"github.com/ShoshinNikita/rview/pkg/cache"
	"github.com/ShoshinNikita/rview/rview"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestThumbnailService(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	const useOriginalImageThresholdSize = 10

	cache, err := cache.NewDiskCache(t.TempDir())
	require.NoError(t, err)

	newService := func(
		t *testing.T,
		openFileFn rview.OpenFileFn,
		resizeFn func(originalFile, cacheFile string, id rview.ThumbnailID) error,
	) *ThumbnailService {

		service := NewThumbnailService(nil, cache, 2, rview.JpegThumbnails)
		service.useOriginalImageThresholdSize = useOriginalImageThresholdSize
		service.openFileFn = openFileFn
		service.resizeFn = resizeFn

		t.Cleanup(func() {
			err = service.Shutdown(ctx)
			require.NoError(t, err)
		})

		return service
	}

	t.Run("resize", func(t *testing.T) {
		r := require.New(t)

		var openFileFnCount, resizedCount int
		service := newService(
			t,
			func(_ context.Context, id rview.FileID) (io.ReadCloser, error) {
				openFileFnCount++
				time.Sleep(110 * time.Millisecond)
				return io.NopCloser(bytes.NewReader([]byte("original-content-" + id.String()))), nil
			},
			func(_, cacheFile string, id rview.ThumbnailID) error {
				resizedCount++
				return os.WriteFile(cacheFile, []byte("resized-content-"+id.GetName()), 0o600)
			},
		)

		fileID := rview.NewFileID("1.jpg", time.Now().Unix(), 0)

		{
			resizeStart := time.Now()

			thumbnailID, err := service.StartThumbnailGeneration(fileID, useOriginalImageThresholdSize+1)
			r.NoError(err)

			// Must ignore in-progress tasks.
			for range 3 {
				_, err = service.StartThumbnailGeneration(fileID, useOriginalImageThresholdSize+1)
				r.NoError(err)
			}

			// Should wait for thumbnail to be ready.
			rc, err := service.OpenThumbnail(ctx, thumbnailID)
			r.NoError(err)

			data, err := io.ReadAll(rc)
			r.NoError(err)
			r.Equal("resized-content-1.thumbnail.jpg", string(data))

			dur := time.Since(resizeStart)
			if dur < 200*time.Millisecond {
				t.Fatalf("image must be opened in >=200ms, got: %s", dur)
			}

			r.Equal(1, openFileFnCount)
			r.Equal(1, resizedCount)
		}

		// Same task - should ignore because thumbnail already exists.
		{
			_, err = service.StartThumbnailGeneration(fileID, useOriginalImageThresholdSize+1)
			r.NoError(err)
			service.inProgressTasksMu.Lock()
			taskCount := len(service.inProgressTasks)
			service.inProgressTasksMu.Unlock()

			r.Zero(taskCount)
		}

		// Same path, but different mod time.
		{
			newFileID := rview.NewFileID(fileID.GetPath(), time.Now().Unix()+5, 0)
			newThumbnailID, err := service.StartThumbnailGeneration(newFileID, useOriginalImageThresholdSize+1)
			r.NoError(err)

			rc, err := service.OpenThumbnail(ctx, newThumbnailID)
			r.NoError(err)
			rc.Close()
			r.Equal(2, openFileFnCount)
			r.Equal(2, resizedCount)
		}

		// Same path, but different size.
		{
			newFileID := rview.NewFileID(fileID.GetPath(), time.Now().Unix(), 15)
			newThumbnailID, err := service.StartThumbnailGeneration(newFileID, useOriginalImageThresholdSize+1)
			r.NoError(err)

			rc, err := service.OpenThumbnail(ctx, newThumbnailID)
			r.NoError(err)
			rc.Close()
			r.Equal(3, openFileFnCount)
			r.Equal(3, resizedCount)
		}
	})

	t.Run("remove resized file after error", func(t *testing.T) {
		r := require.New(t)

		service := newService(
			t,
			func(context.Context, rview.FileID) (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader([]byte("long phrase to exceed threshold"))), nil
			},
			func(_, cacheFile string, thumbnailID rview.ThumbnailID) error {
				// File must be created by vips, emulate it.
				f, err := os.Create(cacheFile)
				r.NoError(err)
				r.NoError(f.Close())

				r.NoError(cache.Check(thumbnailID.FileID))

				return errors.New("some error")
			},
		)

		fileID := rview.NewFileID("2.jpg", time.Now().Unix(), 0)

		thumbnailID, err := service.StartThumbnailGeneration(fileID, useOriginalImageThresholdSize+1)
		r.NoError(err)

		_, err = service.OpenThumbnail(ctx, thumbnailID)
		r.ErrorIs(err, rview.ErrCacheMiss)

		// Cache file must be removed.
		r.ErrorIs(service.cache.Check(fileID), rview.ErrCacheMiss)
	})

	t.Run("use original file", func(t *testing.T) {
		r := require.New(t)

		var resizeCalled bool
		service := newService(
			t,
			func(context.Context, rview.FileID) (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader([]byte("x"))), nil
			},
			func(_, _ string, _ rview.ThumbnailID) error {
				resizeCalled = true
				return errors.New("should not be called")
			},
		)

		fileID := rview.NewFileID("3.jpg", time.Now().Unix(), 0)

		thumbnailID, err := service.StartThumbnailGeneration(fileID, 1)
		r.NoError(err)

		rc, err := service.OpenThumbnail(ctx, thumbnailID)
		r.NoError(err)
		data, err := io.ReadAll(rc)
		r.NoError(err)
		r.Equal("x", string(data))
		r.False(resizeCalled)
	})
}

func TestThumbnailService_CanGenerateThumbnail(t *testing.T) {
	t.Parallel()

	r := require.New(t)

	now := time.Now().Unix()

	canGenerate := NewThumbnailService(nil, nil, 0, rview.JpegThumbnails).CanGenerateThumbnail

	r.True(canGenerate(rview.NewFileID("/home/users/test.png", now, 0)))
	r.True(canGenerate(rview.NewFileID("/home/users/test.pNg", now, 0)))
	r.True(canGenerate(rview.NewFileID("/home/users/test.JPG", now, 0)))
	r.True(canGenerate(rview.NewFileID("/home/users/test with space.jpeg", now, 0)))
	r.True(canGenerate(rview.NewFileID("/test.gif", now, 0)))
	r.False(canGenerate(rview.NewFileID("/home/users/x.txt", now, 0)))
}

func TestThumbnailService_NewThumbnailID(t *testing.T) {
	t.Parallel()

	service := NewThumbnailService(nil, nil, 0, rview.JpegThumbnails)

	for path, wantThumbnail := range map[string]string{
		"/home/cat.jpeg":             "/home/cat.thumbnail.jpeg",
		"/home/abc/qwe/ghj/dog.heic": "/home/abc/qwe/ghj/dog.thumbnail.heic.jpeg",
		"/x/mouse.JPG":               "/x/mouse.thumbnail.JPG",
		"/x/y/z/screenshot.PNG":      "/x/y/z/screenshot.thumbnail.PNG.jpeg",
	} {
		id := rview.NewFileID(path, 33, 15)
		thumbnail, err := service.newThumbnailID(id)
		require.NoError(t, err)
		assert.Equal(t, wantThumbnail, thumbnail.GetPath())
		assert.Equal(t, int64(33), thumbnail.GetModTime().Unix())
		assert.Equal(t, int64(15), thumbnail.GetSize())
	}
}

// TestThumbnailService_ImageType checks that a thumbnail extension matches the actual image type.
func TestThumbnailService_ImageType(t *testing.T) {
	t.Parallel()

	encodeJPEG := func(w, h int) []byte {
		buf := bytes.NewBuffer(nil)
		err := jpeg.Encode(buf, image.NewRGBA(image.Rect(0, 0, w, h)), &jpeg.Options{Quality: 100})
		require.NoError(t, err)
		return buf.Bytes()
	}
	encodePNG := func(w, h int) []byte {
		buf := bytes.NewBuffer(nil)
		enc := png.Encoder{CompressionLevel: png.NoCompression}
		err := enc.Encode(buf, image.NewRGBA(image.Rect(0, 0, w, h)))
		require.NoError(t, err)
		return buf.Bytes()
	}
	type Image struct {
		rawImage []byte
		size     int64
	}
	images := map[string]Image{
		"small.jpeg": {encodeJPEG(100, 100), 791},
		"large.jpg":  {encodeJPEG(8000, 2000), 250595},
		"small.png":  {encodePNG(10, 10), 483},
		"large.png":  {encodePNG(600, 100), 240272},
	}

	checkJPEG := func(t *testing.T, data []byte) {
		require.True(t, bytes.HasPrefix(data, []byte{0xff, 0xd8, 0xff}), "no jpeg signature")
	}
	checkPNG := func(t *testing.T, data []byte) {
		require.True(t, bytes.HasPrefix(data, []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}), "no png signature")
	}
	checkAVIF := func(t *testing.T, data []byte) {
		require.True(t, bytes.Contains(data, []byte("ftypavif")), "no avif signature")
	}
	type Test struct {
		file              string
		wantThumbnailPath string
		checkImageType    func(*testing.T, []byte)
	}
	for _, tt := range []struct {
		thumbnailsFormat rview.ThumbnailsFormat
		tests            []Test
	}{
		{
			thumbnailsFormat: rview.JpegThumbnails,
			tests: []Test{
				{file: "small.jpeg", wantThumbnailPath: "small.jpeg", checkImageType: checkJPEG},
				{file: "large.jpg", wantThumbnailPath: "large.thumbnail.jpg", checkImageType: checkJPEG},
				{file: "small.png", wantThumbnailPath: "small.png", checkImageType: checkPNG},
				{file: "large.png", wantThumbnailPath: "large.thumbnail.png.jpeg", checkImageType: checkJPEG},
			},
		},
		{
			thumbnailsFormat: rview.AvifThumbnails,
			tests: []Test{
				{file: "small.jpeg", wantThumbnailPath: "small.jpeg", checkImageType: checkJPEG},
				{file: "large.jpg", wantThumbnailPath: "large.thumbnail.jpg.avif", checkImageType: checkAVIF},
				{file: "small.png", wantThumbnailPath: "small.png", checkImageType: checkPNG},
				{file: "large.png", wantThumbnailPath: "large.thumbnail.png.avif", checkImageType: checkAVIF},
			},
		},
	} {
		cache, err := cache.NewDiskCache(t.TempDir())
		require.NoError(t, err)

		thumbnailsFormat := tt.thumbnailsFormat
		t.Run(string(tt.thumbnailsFormat), func(t *testing.T) {
			for _, tt := range tt.tests {
				t.Run(tt.file, func(t *testing.T) {
					t.Parallel()

					r := require.New(t)

					img, ok := images[tt.file]
					r.True(ok)
					r.Equal(int(img.size), len(img.rawImage), "wrong image size") //nolint:testifylint

					openFileFn := func(context.Context, rview.FileID) (io.ReadCloser, error) {
						return io.NopCloser(bytes.NewReader(img.rawImage)), nil
					}

					service := NewThumbnailService(openFileFn, cache, 1, thumbnailsFormat)
					thumbnailID, err := service.StartThumbnailGeneration(rview.NewFileID(tt.file, 0, 0), img.size)
					r.NoError(err)
					r.Equal(tt.wantThumbnailPath, thumbnailID.GetPath())

					rc, err := service.OpenThumbnail(context.Background(), thumbnailID)
					r.NoError(err)
					defer rc.Close()
					rawThumbnail, err := io.ReadAll(rc)
					r.NoError(err)
					tt.checkImageType(t, rawThumbnail)
				})
			}
		})
	}
}
