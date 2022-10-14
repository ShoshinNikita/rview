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
	maxHeight = 2048
	maxWidth  = 2048

	jpegQuality = 80
)

type ImageResizer struct {
	dir          string
	workersCount int

	tasksCh chan resizeTask

	stopped       *atomic.Bool
	workersDoneCh chan struct{}
}

type resizeTask struct {
	filename string
	modTime  time.Time
	rc       io.ReadCloser
}

func NewImageResizer(dir string, workersCount int) *ImageResizer {
	r := &ImageResizer{
		dir:          dir,
		workersCount: workersCount,
		//
		tasksCh: make(chan resizeTask, 200),
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
				if err := r.processTask(task); err != nil {
					log.Printf("couldn't process task to resize %q: %s", task.filename, err)
				}
			}
		}()
	}
	wg.Wait()

	close(r.workersDoneCh)
}

func (r *ImageResizer) processTask(task resizeTask) error {
	img, err := r.resize(task)
	if err != nil {
		return fmt.Errorf("couldn't resize image: %w", err)
	}

	path := r.GetFilepath(task.filename, task.modTime)
	err = r.saveImage(path, img)
	if err != nil {
		return fmt.Errorf("couldn't save image: %w", err)
	}
	return nil
}

func (r *ImageResizer) resize(task resizeTask) (image.Image, error) {
	defer task.rc.Close()

	img, err := imaging.Decode(task.rc)
	if err != nil {
		return nil, fmt.Errorf("couldn't decode image: %s", err)
	}

	width, height, shouldResize := thumbnail(img.Bounds(), maxWidth, maxHeight)
	if !shouldResize {
		return img, nil
	}

	return imaging.Resize(img, width, height, imaging.Linear), nil
}

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

func (r *ImageResizer) GetFilepath(filename string, modTime time.Time) string {
	subdir := modTime.Format("2006-01")
	resizedFilename := strconv.Itoa(int(modTime.Unix())) + "_" + filename

	return filepath.Join(r.dir, subdir, resizedFilename)
}

// StartResizing sends a resize task to the queue. It returns an error if the image format
// is not supported (it is detected by filename).
func (r *ImageResizer) StartResizing(filename string, rc io.ReadCloser, modTime time.Time) error {
	if r.stopped.Load() {
		return errors.New("can't send resize tasks after Shutdown call")
	}
	_, err := imaging.FormatFromFilename(filename)
	if err != nil {
		return ErrUnsupportedImageFormat
	}

	r.tasksCh <- resizeTask{
		filename: filename,
		modTime:  modTime,
		rc:       rc,
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
