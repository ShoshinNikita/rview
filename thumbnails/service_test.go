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
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ShoshinNikita/rview/pkg/cache"
	"github.com/ShoshinNikita/rview/rview"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestThumbnailService(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	const useOriginalImageThresholdSize = 10

	cache, err := cache.NewDiskCache(t.TempDir(), cache.Options{DisableCleaner: true})
	require.NoError(t, err)

	newService := func(
		t *testing.T,
		openFileFn func(ctx context.Context, id rview.FileID) (io.ReadCloser, error),
		resizeFn func(originalFile, cacheFile string, id ThumbnailID, size rview.ThumbnailSize) error,
	) *ThumbnailService {

		service := NewThumbnailService(nil, cache, 2, rview.JpegThumbnails)
		service.useOriginalImageThresholdSize = useOriginalImageThresholdSize
		service.rclone = rcloneMock{openFileFn: openFileFn}
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
				time.Sleep(100 * time.Millisecond)
				return io.NopCloser(strings.NewReader(strings.Repeat("x", int(id.GetSize())))), nil
			},
			func(_, cacheFile string, id ThumbnailID, _ rview.ThumbnailSize) error {
				resizedCount++
				return os.WriteFile(cacheFile, []byte("resized-content-"+id.GetName()), 0o600)
			},
		)

		fileID := rview.NewFileID("1.jpg", time.Now().Unix(), useOriginalImageThresholdSize+1)

		{
			resizeStart := time.Now()

			const callCount = 5

			// Must ignore duplicate and in-progress tasks.
			type Res struct {
				rc  io.ReadCloser
				err error
			}
			var (
				resCh = make(chan Res, callCount)
				wg    sync.WaitGroup
			)
			for range callCount {
				wg.Add(1)
				go func() {
					defer wg.Done()
					rc, _, err := service.OpenThumbnail(ctx, fileID, "")
					resCh <- Res{
						rc:  rc,
						err: err,
					}
				}()
			}
			wg.Wait()
			close(resCh)

			for res := range resCh {
				r.NoError(res.err)
				data, err := io.ReadAll(res.rc)
				r.NoError(err)
				r.Equal("resized-content-1.thumbnail.jpg", string(data))
			}

			dur := time.Since(resizeStart)
			if dur < 100*time.Millisecond {
				t.Fatalf("image must be opened in >=100ms, got: %s", dur)
			}

			r.Equal(1, openFileFnCount)
			r.Equal(1, resizedCount)
		}

		// Same task - thumbnail must already exist.
		{
			now := time.Now()
			rc, _, err := service.OpenThumbnail(ctx, fileID, "")
			r.NoError(err)
			rc.Close()

			dur := time.Since(now)
			if dur > 10*time.Millisecond {
				t.Fatalf("OpenThumbnail must finish immediately, took: %s", dur)
			}
		}

		// Same path, but different mod time.
		{
			newFileID := rview.NewFileID(fileID.GetPath(), time.Now().Unix()+5, useOriginalImageThresholdSize+1)

			rc, _, err := service.OpenThumbnail(ctx, newFileID, "")
			r.NoError(err)
			rc.Close()
			r.Equal(2, openFileFnCount)
			r.Equal(2, resizedCount)
		}

		// Same path, but different size.
		{
			newFileID := rview.NewFileID(fileID.GetPath(), fileID.GetModTime().Unix(), useOriginalImageThresholdSize+2)

			rc, _, err := service.OpenThumbnail(ctx, newFileID, "")
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
			func(_, cacheFile string, thumbnailID ThumbnailID, _ rview.ThumbnailSize) error {
				// File must be created by vips, emulate it.
				f, err := os.Create(cacheFile)
				r.NoError(err)
				r.NoError(f.Close())

				rc, err := cache.Open(thumbnailID.FileID)
				r.NoError(err)
				rc.Close()

				return errors.New("some error")
			},
		)

		fileID := rview.NewFileID("2.jpg", time.Now().Unix(), useOriginalImageThresholdSize+1)

		_, _, err = service.OpenThumbnail(ctx, fileID, "")
		r.ErrorIs(err, rview.ErrCacheMiss)

		// Cache file must be removed.
		_, err := service.cache.Open(fileID)
		r.ErrorIs(err, rview.ErrCacheMiss)
	})

	t.Run("use original file", func(t *testing.T) {
		r := require.New(t)

		var resizeCalled bool
		service := newService(
			t,
			func(context.Context, rview.FileID) (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader([]byte("x"))), nil
			},
			func(_, _ string, _ ThumbnailID, _ rview.ThumbnailSize) error {
				resizeCalled = true
				return errors.New("should not be called")
			},
		)

		fileID := rview.NewFileID("3.jpg", time.Now().Unix(), 1)

		rc, _, err := service.OpenThumbnail(ctx, fileID, "")
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
		"/home/abc/qwe/ghj/dog.heic": "/home/abc/qwe/ghj/dog.heic.thumbnail.jpeg",
		"/x/mouse.JPG":               "/x/mouse.thumbnail.JPG",
		"/x/y/z/screenshot.PNG":      "/x/y/z/screenshot.PNG.thumbnail.jpeg",
	} {
		id := rview.NewFileID(path, 33, 15)
		thumbnail, err := service.newThumbnailID(id, "")
		require.NoError(t, err)
		assert.Equal(t, wantThumbnail, thumbnail.GetPath())
		assert.Equal(t, int64(33), thumbnail.GetModTime().Unix())
		assert.Equal(t, int64(15), thumbnail.GetSize())
	}
}

// TestThumbnailService_ImageType checks that content type matches the actual image type.
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
		file            string
		wantContentType string
		checkImageType  func(*testing.T, []byte)
	}
	for _, tt := range []struct {
		thumbnailsFormat rview.ThumbnailsFormat
		tests            []Test
	}{
		{
			thumbnailsFormat: rview.JpegThumbnails,
			tests: []Test{
				{file: "small.jpeg", wantContentType: "image/jpeg", checkImageType: checkJPEG},
				{file: "large.jpg", wantContentType: "image/jpeg", checkImageType: checkJPEG},
				{file: "small.png", wantContentType: "image/png", checkImageType: checkPNG},
				{file: "large.png", wantContentType: "image/jpeg", checkImageType: checkJPEG},
			},
		},
		{
			thumbnailsFormat: rview.AvifThumbnails,
			tests: []Test{
				{file: "small.jpeg", wantContentType: "image/jpeg", checkImageType: checkJPEG},
				{file: "large.jpg", wantContentType: "image/avif", checkImageType: checkAVIF},
				{file: "small.png", wantContentType: "image/png", checkImageType: checkPNG},
				{file: "large.png", wantContentType: "image/avif", checkImageType: checkAVIF},
			},
		},
	} {
		cache, err := cache.NewDiskCache(t.TempDir(), cache.Options{DisableCleaner: true})
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

					rclone := rcloneMock{
						openFileFn: func(context.Context, rview.FileID) (io.ReadCloser, error) {
							return io.NopCloser(bytes.NewReader(img.rawImage)), nil
						},
					}
					service := NewThumbnailService(rclone, cache, 1, thumbnailsFormat)

					fileID := rview.NewFileID(tt.file, 0, img.size)
					rc, contentType, err := service.OpenThumbnail(t.Context(), fileID, "")
					r.NoError(err)
					defer rc.Close()
					rawThumbnail, err := io.ReadAll(rc)
					r.NoError(err)
					tt.checkImageType(t, rawThumbnail)
					r.Equal(tt.wantContentType, contentType)
				})
			}
		})
	}
}

// TestThumbnailService_AllImageTypes checks that we can successfully generate
// thumbnails for all supported image types.
func TestThumbnailService_AllImageTypes(t *testing.T) {
	cache, err := cache.NewDiskCache(t.TempDir(), cache.Options{DisableCleaner: true})
	require.NoError(t, err)

	type Test struct {
		imageType       string
		file            string
		wantContentType string
		sameSize        bool
	}
	runTests := func(t *testing.T, format rview.ThumbnailsFormat, tests []Test) {
		for _, tt := range tests {
			t.Run(tt.imageType, func(t *testing.T) {
				r := require.New(t)

				originalImage, err := os.ReadFile(filepath.Join("../tests/testdata", tt.file))
				r.NoError(err)

				mock := rcloneMock{
					openFileFn: func(context.Context, rview.FileID) (io.ReadCloser, error) {
						return io.NopCloser(bytes.NewReader(originalImage)), nil
					},
				}
				thumbnailService := NewThumbnailService(mock, cache, 1, format)
				thumbnailService.GenerateThumbnailsForSmallFiles()

				ctx, cancel := context.WithTimeout(t.Context(), time.Second)
				defer cancel()

				fileID := rview.NewFileID(tt.file, 0, int64(len(originalImage)))
				rc, contentType, err := thumbnailService.OpenThumbnail(ctx, fileID, "")
				r.NoError(err)
				defer rc.Close()

				thumbnail, err := io.ReadAll(rc)
				r.NoError(err)
				if tt.sameSize {
					r.Equal(len(thumbnail), len(originalImage), "size of thumbnail and original file should be equal")
				} else {
					r.NotEqual(len(thumbnail), len(originalImage), "size of thumbnail and original file should differ")
				}

				r.Equal(tt.wantContentType, contentType)
			})
		}
	}

	t.Run("jpeg", func(t *testing.T) {
		runTests(t, rview.JpegThumbnails, []Test{
			{imageType: "jpg", file: "Images/birds-g64b44607c_640.jpg", wantContentType: "image/jpeg"},
			{imageType: "png", file: "Images/ytrewq.png", wantContentType: "image/jpeg"},
			{imageType: "webp", file: "Images/qwerty.webp", wantContentType: "image/webp"},
			{imageType: "heic", file: "Images/asdfgh.heic", wantContentType: "image/jpeg"}, // we should generate .jpeg thumbnails for .heic images
			{imageType: "avif", file: "Images/sky.avif", wantContentType: "image/avif"},
			{imageType: "gif", file: "test.gif", wantContentType: "image/gif", sameSize: true}, // we save the original file
		})
	})

	t.Run("avif", func(t *testing.T) {
		runTests(t, rview.AvifThumbnails, []Test{
			{imageType: "jpg", file: "Images/birds-g64b44607c_640.jpg", wantContentType: "image/avif"},
			{imageType: "png", file: "Images/ytrewq.png", wantContentType: "image/avif"},
			{imageType: "webp", file: "Images/qwerty.webp", wantContentType: "image/webp"},
			{imageType: "heic", file: "Images/asdfgh.heic", wantContentType: "image/avif"}, // we should generate .avif thumbnails for .heic images
			{imageType: "avif", file: "Images/sky.avif", wantContentType: "image/avif"},
			{imageType: "gif", file: "test.gif", wantContentType: "image/gif", sameSize: true}, // we save the original file
		})
	})
}

type rcloneMock struct {
	openFileFn func(context.Context, rview.FileID) (io.ReadCloser, error)
}

func (x rcloneMock) OpenFile(ctx context.Context, id rview.FileID) (io.ReadCloser, error) {
	return x.openFileFn(ctx, id)
}
