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

	"github.com/ShoshinNikita/rview/metrics"
	"github.com/ShoshinNikita/rview/rlog"
	"github.com/ShoshinNikita/rview/rview"
)

var (
	ErrUnsupportedImageFormat = errors.New("unsupported image format")
)

type ImageResizer struct {
	cache rview.Cache

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

func NewImageResizer(cache rview.Cache, workersCount int) *ImageResizer {
	r := &ImageResizer{
		cache: cache,
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

				if err != nil {
					metrics.ResizerErrors.Inc()
					rlog.Errorf("couldn't process task to resize %q: %s", task.GetPath(), err)
				} else {
					metrics.ResizerDownloadedImageSizes.Observe(float64(stats.originalSize))
					rlog.Debugf(
						"file %q was resized in %s, original size: %.2f MiB, new size: %.2f MiB",
						task.FileID, dur, float64(stats.originalSize)/(1<<20), float64(stats.resizedSize)/(1<<20),
					)
				}

				metrics.ResizerProcessDuration.Observe(dur.Seconds())

				cancel()
				delete(r.inProgressTasks, task.FileID)
			}
		}()
	}
	wg.Wait()

	close(r.workersDoneCh)
}

type stats struct {
	originalSize int64
	resizedSize  int64
}

func (r *ImageResizer) processTask(ctx context.Context, task resizeTask) (stats, error) {
	rc, err := task.openFileFn(ctx, task.FileID)
	if err != nil {
		return stats{}, fmt.Errorf("couldn't get image reader: %w", err)
	}
	defer rc.Close()

	// We could pass file reader as stdin of [exec.Cmd], but it caused weird errors:
	//
	//	VipsJpeg: Corrupt JPEG data: premature end of data segment
	//
	// An approach with temp files works well, so stick to it.
	tempFile, err := os.CreateTemp("", "rview-*")
	if err != nil {
		return stats{}, fmt.Errorf("couldn't create temp image file: %w", err)
	}
	defer func() {
		if err := tempFile.Close(); err != nil {
			rlog.Errorf("couldn't close temp image file: %s", err)
		}
		if err := os.Remove(tempFile.Name()); err != nil {
			rlog.Errorf("couldn't remove temp image file: %s", err)
		}
	}()

	originalSize, err := io.Copy(tempFile, rc)
	if err != nil {
		return stats{}, fmt.Errorf("couldn't load image: %w", err)
	}

	w, err := r.cache.GetSaveWriter(task.FileID)
	if err != nil {
		return stats{}, fmt.Errorf("couldn't get cache writer: %w", err)
	}
	defer w.Close()

	writeCounter := &readWriteCounter{w: w}

	// https://www.libvips.org/API/current/Using-vipsthumbnail.html
	cmd := exec.Command(
		"vips",
		"thumbnail_source",
		"[descriptor=0]",
		".jpg[Q=80,optimize_coding,strip]",
		"1024",
	)
	stderr := bytes.NewBuffer(nil)
	cmd.Stdin = tempFile
	cmd.Stdout = writeCounter
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		return stats{}, fmt.Errorf("couldn't resize image: %w, stderr: %q", err, stderr.String())
	}
	if stderr.Len() > 0 {
		rlog.Infof("vips stderr for %q: %q", task.FileID, stderr.String())
	}

	return stats{
		originalSize: originalSize,
		resizedSize:  writeCounter.size,
	}, nil
}

// CanResize detects if a file can be resized based on its filename.
func (r *ImageResizer) CanResize(id rview.FileID) bool {
	ext := strings.ToLower(filepath.Ext(id.GetName()))
	switch ext {
	case ".jpg", ".jpeg", ".png":
		return true
	default:
		return false
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

	ticker := time.NewTicker(500 * time.Millisecond)
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
// should be absolute.
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

type readWriteCounter struct {
	r    io.Reader
	w    io.Writer
	size int64
}

func (rw *readWriteCounter) Read(p []byte) (int, error) {
	n, err := rw.r.Read(p)
	rw.size += int64(n)
	return n, err
}

func (rw *readWriteCounter) Write(p []byte) (int, error) {
	n, err := rw.w.Write(p)
	rw.size += int64(n)
	return n, err
}
