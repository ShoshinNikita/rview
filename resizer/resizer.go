package resizer

import (
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/disintegration/imaging"
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
	dir          string
	workersCount int

	tasksCh           chan resizeTask
	inProgressTasks   map[taskID]struct{}
	inProgressTasksMu sync.RWMutex

	stopped       *atomic.Bool
	workersDoneCh chan struct{}
}

// GetFileFn is used to download file.
type GetFileFn func(ctx context.Context, filepath string) (io.ReadCloser, error)

type resizeTask struct {
	taskID

	getFileFn GetFileFn
}

type taskID struct {
	filepath string
	modTime  int64 // unix time
}

func NewImageResizer(dir string, workersCount int) *ImageResizer {
	r := &ImageResizer{
		dir:          dir,
		workersCount: workersCount,
		//
		tasksCh:         make(chan resizeTask, 200),
		inProgressTasks: make(map[taskID]struct{}),
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

				if err := r.processTask(ctx, task); err != nil {
					log.Printf("couldn't process task to resize %q: %s", task.filepath, err)
				}

				cancel()
				delete(r.inProgressTasks, task.taskID)
			}
		}()
	}
	wg.Wait()

	close(r.workersDoneCh)
}

func (r *ImageResizer) processTask(ctx context.Context, task resizeTask) error {
	img, err := r.resize(ctx, task)
	if err != nil {
		return fmt.Errorf("couldn't resize image: %w", err)
	}

	path := r.generateFilepath(task.filepath, time.Unix(task.modTime, 0))
	err = r.saveImage(path, img)
	if err != nil {
		return fmt.Errorf("couldn't save image: %w", err)
	}
	return nil
}

// resize downloads a file and resize it. It can return the original image if resizing is not needed.
func (r *ImageResizer) resize(ctx context.Context, task resizeTask) (image.Image, error) {
	rc, err := task.getFileFn(ctx, task.filepath)
	if err != nil {
		return nil, fmt.Errorf("couldn't get image reader: %w", err)
	}
	defer rc.Close()

	img, err := imaging.Decode(rc)
	if err != nil {
		return nil, fmt.Errorf("couldn't decode image: %s", err)
	}

	width, height, shouldResize := thumbnail(img.Bounds(), maxWidth, maxHeight)
	if !shouldResize {
		return img, nil
	}

	return imaging.Resize(img, width, height, imaging.Linear), nil
}

// saveImage saves an image. It creates all directories if needed.
func (r *ImageResizer) saveImage(path string, img image.Image) error {
	dir := filepath.Dir(path)
	err := os.MkdirAll(dir, 0o777)
	if err != nil {
		return fmt.Errorf("couldn't create dir %q: %w", dir, err)
	}

	format, err := imaging.FormatFromFilename(path)
	if err != nil {
		return fmt.Errorf("couldn't determine image format: %w", err)
	}

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("couldn't create file %q: %w", path, err)
	}
	defer file.Close()

	err = imaging.Encode(file, img, format, imaging.JPEGQuality(jpegQuality))
	if err != nil {
		return fmt.Errorf("couldn't encode image: %w", err)
	}
	return nil
}

// CanResize detects if a file can be resized based on its filename.
func (r *ImageResizer) CanResize(filename string) bool {
	_, err := imaging.FormatFromFilename(filename)
	return err == nil
}

// IsResized returns true if this file is already resized or is in the task queue.
func (r *ImageResizer) IsResized(filepath string, modTime time.Time) bool {
	id := taskID{
		filepath: filepath,
		modTime:  modTime.Unix(),
	}
	r.inProgressTasksMu.RLock()
	_, inProgress := r.inProgressTasks[id]
	r.inProgressTasksMu.RUnlock()
	if inProgress {
		return true
	}

	path := r.generateFilepath(filepath, modTime)
	_, err := os.Stat(path)
	return err == nil
}

// OpenResized returns io.ReadCloser for the resized image. It waits for the files in queue, but no longer
// than context timeout.
func (r *ImageResizer) OpenResized(ctx context.Context, filepath string, modTime time.Time) (io.ReadCloser, error) {
	id := taskID{
		filepath: filepath,
		modTime:  modTime.Unix(),
	}
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

	path := r.generateFilepath(filepath, modTime)
	return os.Open(path)
}

// Resize sends a resize task to the queue. It returns an error if the image format
// is not supported (it is detected by filepath). Filepath is passed to getImageFn, so it
// should be absolute.
//
// Resize ignores duplicate tasks. However, it doesn't check files on disk.
func (r *ImageResizer) Resize(filepath string, modTime time.Time, getImageFn GetFileFn) error {
	if r.stopped.Load() {
		return errors.New("can't send resize tasks after Shutdown call")
	}
	if !r.CanResize(filepath) {
		return ErrUnsupportedImageFormat
	}

	id := taskID{
		filepath: filepath,
		modTime:  modTime.Unix(),
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
		taskID:    id,
		getFileFn: getImageFn,
	}
	return nil
}

// generateFilepath generates a filepath of pattern '<dir>/<YYYY-MM>/<modTime>_<filename>'.
func (r *ImageResizer) generateFilepath(srcFilepath string, modTime time.Time) string {
	subdir := modTime.Format("2006-01")
	filename := filepath.Base(srcFilepath)
	resizedFilename := strconv.Itoa(int(modTime.Unix())) + "_" + filename

	return filepath.Join(r.dir, subdir, resizedFilename)
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
