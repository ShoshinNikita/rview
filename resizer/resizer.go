package resizer

import (
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/disintegration/imaging"

	"github.com/ShoshinNikita/rview/rlog"
	"github.com/ShoshinNikita/rview/rview"
)

var (
	ErrUnsupportedImageFormat = errors.New("unsupported image format")
)

const (
	maxHeight = 1024
	maxWidth  = 1024

	jpegQuality = 80
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

				stats, err := r.processTask(ctx, task)
				if err != nil {
					rlog.Errorf("couldn't process task to resize %q: %s", task.GetPath(), err)
				} else {
					rlog.Debugf(
						"file %q was resized, original size: %.2f MiB, new size: %.2f MiB",
						task.FileID, float64(stats.originalSize)/(1<<20), float64(stats.resizedSize)/(1<<20),
					)
				}

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
	img, originalSize, err := r.resize(ctx, task)
	if err != nil {
		return stats{}, fmt.Errorf("couldn't resize image: %w", err)
	}

	resizedSize, err := r.saveImage(task.FileID, img)
	if err != nil {
		return stats{}, fmt.Errorf("couldn't save image: %w", err)
	}
	return stats{
		originalSize: originalSize,
		resizedSize:  resizedSize,
	}, nil
}

// resize downloads a file and resize it. It can return the original image if resizing is not needed.
func (r *ImageResizer) resize(ctx context.Context, task resizeTask) (_ image.Image, originalSize int64, _ error) {
	rc, err := task.openFileFn(ctx, task.FileID)
	if err != nil {
		return nil, 0, fmt.Errorf("couldn't get image reader: %w", err)
	}
	defer rc.Close()

	readCounter := &readWriteCounter{r: rc}
	img, err := imaging.Decode(readCounter)
	if err != nil {
		return nil, 0, fmt.Errorf("couldn't decode image: %w", err)
	}

	width, height, shouldResize := thumbnail(img.Bounds(), maxWidth, maxHeight)
	if shouldResize {
		img = imaging.Resize(img, width, height, imaging.Linear)
	}
	return img, readCounter.size, nil
}

// saveImage saves an image. It creates all directories if needed.
func (r *ImageResizer) saveImage(id rview.FileID, img image.Image) (resizedSize int64, _ error) {
	format, err := imaging.FormatFromFilename(id.GetName())
	if err != nil {
		return 0, fmt.Errorf("couldn't determine image format: %w", err)
	}

	w, err := r.cache.GetSaveWriter(id)
	if err != nil {
		return 0, fmt.Errorf("couldn't get cache writer: %w", err)
	}
	defer w.Close()

	writeCounter := &readWriteCounter{w: w}
	err = imaging.Encode(writeCounter, img, format, imaging.JPEGQuality(jpegQuality))
	if err != nil {
		return 0, fmt.Errorf("couldn't encode image: %w", err)
	}
	return writeCounter.size, nil
}

// CanResize detects if a file can be resized based on its filename.
func (r *ImageResizer) CanResize(id rview.FileID) bool {
	_, err := imaging.FormatFromFilename(id.GetName())
	return err == nil
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
