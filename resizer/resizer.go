package resizer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ShoshinNikita/rview/pkg/metrics"
	"github.com/ShoshinNikita/rview/pkg/rlog"
	"github.com/ShoshinNikita/rview/rview"
)

type imageType int

const (
	unsupportedImageType imageType = iota
	jpegImageType
	pngImageType
	gifImageType
)

var (
	ErrUnsupportedImageFormat = errors.New("unsupported image format")
)

type ImageResizer struct {
	cache    rview.Cache
	resizeFn func(originalFile, cacheFile string, id rview.FileID) error
	// useOriginalImageThresholdSize defines the maximum size of an original image that should be
	// used without resizing. The main purpose of resizing is to reduce image size, and with small
	// files it is not always possible - after resizing they become just larger.
	useOriginalImageThresholdSize int64

	workersCount int

	tasksCh           chan resizeTask
	inProgressTasks   map[rview.FileID]struct{}
	inProgressTasksMu sync.RWMutex

	stopped       *atomic.Bool
	workersDoneCh chan struct{}
}

type resizeTask struct {
	rview.FileID

	openFileFn rview.OpenFileFn
}

func CheckVips() error {
	cmd := exec.Command("vips", "--version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("vips is not installed: %w", err)
	}
	return nil
}

func NewImageResizer(cache rview.Cache, workersCount int) *ImageResizer {
	r := &ImageResizer{
		cache:                         cache,
		resizeFn:                      resizeWithVips,
		useOriginalImageThresholdSize: 200 << 10, // 200 KiB
		//
		workersCount: workersCount,
		//
		tasksCh:         make(chan resizeTask, 200),
		inProgressTasks: make(map[rview.FileID]struct{}),
		//
		stopped:       new(atomic.Bool),
		workersDoneCh: make(chan struct{}),
	}

	go r.startWorkers()

	return r
}

func (r *ImageResizer) startWorkers() {
	toMiB := func(v int64) string {
		return fmt.Sprintf("%.3f MiB", float64(v)/(1<<20))
	}

	var wg sync.WaitGroup
	for i := 0; i < r.workersCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for task := range r.tasksCh {
				ctx, cancel := context.WithTimeout(context.Background(), time.Minute)

				now := time.Now()
				stats, err := r.processTask(ctx, task)
				dur := time.Since(now)

				if stats.originalSize > 0 {
					metrics.ResizerDownloadedImageSizes.Observe(float64(stats.originalSize))
				}

				switch {
				case err != nil:
					metrics.ResizerErrors.Inc()
					rlog.Errorf("couldn't process task to resize %q: %s", task.GetPath(), err)

				case stats.originalImageUsed:
					metrics.ResizerOriginalImageUsed.Inc()
					rlog.Debugf("use original image for %q, size: %s", task.GetPath(), toMiB(stats.originalSize))

				default:
					metrics.ResizerProcessDuration.Observe(dur.Seconds())
					metrics.ResizerSizeRatio.Observe(float64(stats.originalSize) / float64(stats.resizedSize))

					msg := fmt.Sprintf(
						"file %q was resized in %s, original size: %s, new size: %s",
						task.GetPath(), dur, toMiB(stats.originalSize), toMiB(stats.resizedSize),
					)

					const reportThreshold = 10 << 10 // 10 Kib

					if diff := stats.resizedSize - stats.originalSize; diff > reportThreshold {
						rlog.Warnf("resized file is greater than the original one by %s: %s", toMiB(diff), msg)
					} else {
						rlog.Debug(msg)
					}
				}

				cancel()

				r.inProgressTasksMu.Lock()
				delete(r.inProgressTasks, task.FileID)
				r.inProgressTasksMu.Unlock()
			}
		}()
	}
	wg.Wait()

	close(r.workersDoneCh)
}

type stats struct {
	originalSize      int64
	resizedSize       int64
	originalImageUsed bool
}

func (r *ImageResizer) processTask(ctx context.Context, task resizeTask) (finalStats stats, err error) {
	rc, err := task.openFileFn(ctx, task.FileID)
	if err != nil {
		return stats{}, fmt.Errorf("couldn't get image reader: %w", err)
	}
	defer rc.Close()

	tempFile, err := os.CreateTemp("", "rview-*")
	if err != nil {
		return stats{}, fmt.Errorf("couldn't create temp image file: %w", err)
	}
	defer func() {
		if err := tempFile.Close(); err != nil {
			rlog.Errorf("couldn't close temp image file: %s", err)

			// Don't exit - try to remove the temp file.
		}
		if err := os.Remove(tempFile.Name()); err != nil {
			rlog.Errorf("couldn't remove temp image file: %s", err)
		}
	}()

	originalSize, err := io.Copy(tempFile, rc)
	if err != nil {
		return stats{}, fmt.Errorf("couldn't load image: %w", err)
	}

	// Don't close temp file right after the copy operation because we still may use it.

	cacheFilepath, err := r.cache.GetFilepath(task.FileID)
	if err != nil {
		return stats{}, fmt.Errorf("couldn't get path of a cache file: %w", err)
	}

	saveOriginal := false ||
		// It doesn't make much sense to resize small files.
		originalSize < r.useOriginalImageThresholdSize ||
		// Save the original file because vipsthumbnail can't resize gifs:
		// https://github.com/libvips/libvips/issues/61#issuecomment-168169916
		getImageType(task.FileID) == gifImageType

	if saveOriginal {
		err := createCacheFileFromTempFile(tempFile, cacheFilepath, originalSize)
		if err != nil {
			return stats{}, err
		}

		return stats{
			originalSize:      originalSize,
			resizedSize:       originalSize,
			originalImageUsed: true,
		}, nil
	}

	err = r.resizeFn(tempFile.Name(), cacheFilepath, task.FileID)
	if err != nil {
		if err := r.cache.Remove(task.FileID); err != nil {
			rlog.Errorf("couldn't remove cache file for %s after resize error: %s", task.FileID, err)
		}

		return stats{}, err
	}

	info, err := os.Stat(cacheFilepath)
	if err != nil {
		return stats{}, fmt.Errorf("couldn't get stats of a cache file: %w", err)
	}
	return stats{
		originalSize: originalSize,
		resizedSize:  info.Size(),
	}, nil
}

// createCacheFileFromTempFile creates a cache file reading the content from a passed temp file.
// We can't just use [os.Rename] because rename operation can fail in docker containers, see
// https://stackoverflow.com/q/42392600/7752659.
func createCacheFileFromTempFile(tempFile *os.File, cacheFilepath string, originalSize int64) error {
	_, err := tempFile.Seek(0, 0)
	if err != nil {
		return fmt.Errorf("couldn't seek temp file: %w", err)
	}

	cacheFile, err := os.Create(cacheFilepath)
	if err != nil {
		return fmt.Errorf("couldn't create cache file: %w", err)
	}

	copied, err := io.Copy(cacheFile, tempFile)
	if err != nil {
		return fmt.Errorf("couldn't copy temp file content to a cache file: %w", err)
	}
	if copied != originalSize {
		return fmt.Errorf("not all content was copied, original size: %d, copied: %d", originalSize, copied)
	}

	if err := cacheFile.Close(); err != nil {
		return fmt.Errorf("couldn't close cache file: %w", err)
	}
	return nil
}

// resizeWithVips resizes the original file with "vipsthumbnail" command. We can't use
// "vips thumbnail_source" because it doesn't support conditional resizing (> or < after
// the size). Without conditional resizing we could get resized images that are larger
// than the original ones.
//
// See https://www.libvips.org/API/current/Using-vipsthumbnail.html for "vipsthumbnail" docs.
func resizeWithVips(originalFile, cacheFile string, fileID rview.FileID) error {
	output := cacheFile
	switch getImageType(fileID) {
	case jpegImageType:
		output += "[Q=80,optimize_coding,strip]"
	case pngImageType:
		output += "[strip]"
	default:
		return errors.New("unsupported image type")
	}

	cmd := exec.Command(
		"vipsthumbnail",
		"--rotate", // auto-rotate
		originalFile,
		"--size", "1024>",
		"-o", output,
	)
	stderr := bytes.NewBuffer(nil)
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("couldn't resize image: %w, stderr: %q", err, stderr.String())
	}
	if stderr.Len() > 0 {
		rlog.Infof("vips stderr for %q: %q", fileID, stderr.String())
	}
	return nil
}

// CanResize detects if a file can be resized based on its filename.
func (r *ImageResizer) CanResize(id rview.FileID) bool {
	return getImageType(id) != unsupportedImageType
}

func getImageType(id rview.FileID) imageType {
	ext := strings.ToLower(filepath.Ext(id.GetName()))
	switch ext {
	case ".jpg", ".jpeg":
		return jpegImageType
	case ".png":
		return pngImageType
	case ".gif":
		return gifImageType
	default:
		return unsupportedImageType
	}
}

// IsResized returns true if this file is already resized or is in the task queue.
func (r *ImageResizer) IsResized(id rview.FileID) bool {
	r.inProgressTasksMu.RLock()
	_, inProgress := r.inProgressTasks[id]
	r.inProgressTasksMu.RUnlock()
	if inProgress {
		return true
	}

	return r.cache.Check(id) == nil
}

// OpenResized returns io.ReadCloser for the resized image. It waits for the files in queue, but no longer
// than context timeout.
func (r *ImageResizer) OpenResized(ctx context.Context, id rview.FileID) (io.ReadCloser, error) {
	isInProgress := func() (inProgress bool) {
		r.inProgressTasksMu.RLock()
		defer r.inProgressTasksMu.RUnlock()

		_, inProgress = r.inProgressTasks[id]
		return inProgress
	}

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		// Check immediately
		if !isInProgress() {
			break
		}

		select {
		case <-ticker.C:
			continue
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return r.cache.Open(id)
}

// Resize sends a resize task to the queue. It returns an error if the image format
// is not supported (it is detected by filepath). Filepath is passed to getImageFn, so it
// must be absolute.
//
// Resize ignores duplicate tasks. However, it doesn't check files on disk.
func (r *ImageResizer) Resize(id rview.FileID, openFileFn rview.OpenFileFn) error {
	if r.stopped.Load() {
		return errors.New("can't send resize tasks after Shutdown call")
	}
	if !r.CanResize(id) {
		return ErrUnsupportedImageFormat
	}

	var ignore bool
	func() {
		r.inProgressTasksMu.Lock()
		defer r.inProgressTasksMu.Unlock()

		if _, ok := r.inProgressTasks[id]; ok {
			ignore = true
			return
		}
		r.inProgressTasks[id] = struct{}{}
	}()
	if ignore {
		return nil
	}

	r.tasksCh <- resizeTask{
		FileID:     id,
		openFileFn: openFileFn,
	}
	return nil
}

// Shutdown drops all tasks in the queue and waits for ones that are in progress
// with respect of the passed context.
func (r *ImageResizer) Shutdown(ctx context.Context) error {
	r.stopped.Store(true)

	close(r.tasksCh)
	for range r.tasksCh {
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.workersDoneCh:
		return nil
	}
}
