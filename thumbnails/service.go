package thumbnails

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"os/exec"
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
	webpImageType
	heicImageType
)

var (
	ErrUnsupportedImageFormat = errors.New("unsupported image format")
)

type ThumbnailService struct {
	cache    rview.Cache
	resizeFn func(originalFile, cacheFile string, id rview.FileID) error
	// useOriginalImageThresholdSize defines the maximum size of an original image that should be
	// used without resizing. The main purpose of resizing is to reduce image size, and with small
	// files it is not always possible - after resizing they become just larger.
	useOriginalImageThresholdSize int64

	workersCount int

	tasksCh           chan generateThumbnailTask
	inProgressTasks   map[rview.FileID]struct{}
	inProgressTasksMu sync.RWMutex

	stopped       *atomic.Bool
	workersDoneCh chan struct{}
}

type generateThumbnailTask struct {
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

// NewThumbnailService prepares a new service for thumbnail generation.
//
// By default we generate thumbnails only for large files and use
// the small ones as-is. It is possible to change this behavior by passing
// generateThumbnailsForSmallFiles = true.
//
// For some images we can generate thumbnails of different formats. For example,
// for .heic images we generate .jpeg thumbnails.
func NewThumbnailService(cache rview.Cache, workersCount int, generateThumbnailsForSmallFiles bool) *ThumbnailService {
	r := &ThumbnailService{
		cache:                         &cacheWrapper{cache},
		resizeFn:                      resizeWithVips,
		useOriginalImageThresholdSize: 200 << 10, // 200 KiB
		//
		workersCount: workersCount,
		//
		tasksCh:         make(chan generateThumbnailTask, 200),
		inProgressTasks: make(map[rview.FileID]struct{}),
		//
		stopped:       new(atomic.Bool),
		workersDoneCh: make(chan struct{}),
	}
	if generateThumbnailsForSmallFiles {
		r.useOriginalImageThresholdSize = 0
	}

	go r.startWorkers()

	return r
}

func (s *ThumbnailService) startWorkers() {
	toMiB := func(v int64) string {
		return fmt.Sprintf("%.3f MiB", float64(v)/(1<<20))
	}

	var wg sync.WaitGroup
	for i := 0; i < s.workersCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for task := range s.tasksCh {
				ctx, cancel := context.WithTimeout(context.Background(), time.Minute)

				now := time.Now()
				stats, err := s.processTask(ctx, task)
				dur := time.Since(now)

				if stats.originalSize > 0 {
					metrics.ThumbnailsOriginalImageSizes.Observe(float64(stats.originalSize))
				}

				switch {
				case err != nil:
					metrics.ThumbnailsErrors.Inc()
					rlog.Errorf("couldn't process task for %q: %s", task.GetPath(), err)

				case stats.originalImageUsed:
					metrics.ThumbnailsOriginalImageUsed.Inc()
					rlog.Debugf("use original image for %q, size: %s", task.GetPath(), toMiB(stats.originalSize))

				default:
					metrics.ThumbnailsProcessTaskDuration.Observe(dur.Seconds())
					metrics.ThumbnailsSizeRatio.Observe(float64(stats.originalSize) / float64(stats.thumbnailSize))

					msg := fmt.Sprintf(
						"thumbnail for %q was generated in %s, original size: %s, new size: %s",
						task.GetPath(), dur, toMiB(stats.originalSize), toMiB(stats.thumbnailSize),
					)

					const reportThreshold = 10 << 10 // 10 Kib

					if diff := stats.thumbnailSize - stats.originalSize; diff > reportThreshold {
						rlog.Warnf("thumbnail is greater than the original file by %s: %s", toMiB(diff), msg)
					} else {
						rlog.Debug(msg)
					}
				}

				cancel()

				s.inProgressTasksMu.Lock()
				delete(s.inProgressTasks, task.FileID)
				s.inProgressTasksMu.Unlock()
			}
		}()
	}
	wg.Wait()

	close(s.workersDoneCh)
}

type stats struct {
	originalSize      int64
	thumbnailSize     int64
	originalImageUsed bool
}

func (s *ThumbnailService) processTask(ctx context.Context, task generateThumbnailTask) (finalStats stats, err error) {
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

	cacheFilepath, err := s.cache.GetFilepath(task.FileID)
	if err != nil {
		return stats{}, fmt.Errorf("couldn't get path of a cache file: %w", err)
	}

	imageType := getImageType(task.FileID)

	var saveOriginal bool
	// It doesn't make much sense to resize small files.
	saveOriginal = saveOriginal || originalSize < s.useOriginalImageThresholdSize
	// Save the original file because vipsthumbnail can't resize gifs:
	// https://github.com/libvips/libvips/issues/61#issuecomment-168169916
	saveOriginal = saveOriginal || imageType == gifImageType
	// We have to always generate thumbnails for .heic because most browsers don't support it.
	saveOriginal = saveOriginal && imageType != heicImageType

	if saveOriginal {
		err := createCacheFileFromTempFile(tempFile, cacheFilepath, originalSize)
		if err != nil {
			return stats{}, err
		}

		return stats{
			originalSize:      originalSize,
			thumbnailSize:     originalSize,
			originalImageUsed: true,
		}, nil
	}

	err = s.resizeFn(tempFile.Name(), cacheFilepath, task.FileID)
	if err != nil {
		if err := s.cache.Remove(task.FileID); err != nil {
			rlog.Errorf("couldn't remove thumbnail for %s after resize error: %s", task.FileID, err)
		}

		return stats{}, err
	}

	info, err := os.Stat(cacheFilepath)
	if err != nil {
		return stats{}, fmt.Errorf("couldn't get stats of a cache file: %w", err)
	}
	return stats{
		originalSize:  originalSize,
		thumbnailSize: info.Size(),
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
	case heicImageType:
		// We generate .jpeg thumbnails for .heic images.
		fallthrough
	case jpegImageType:
		output += "[Q=80,optimize_coding,strip]"
	case pngImageType:
		output += "[strip]"
	case webpImageType:
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

// CanGenerateThumbnail detects if we can generate a thumbnail for a file based on its filename.
func (*ThumbnailService) CanGenerateThumbnail(id rview.FileID) bool {
	return getImageType(id) != unsupportedImageType
}

func getImageType(id rview.FileID) imageType {
	switch id.GetExt() {
	case ".jpg", ".jpeg":
		return jpegImageType
	case ".png":
		return pngImageType
	case ".gif":
		return gifImageType
	case ".webp":
		return webpImageType
	case ".heic":
		return heicImageType
	default:
		return unsupportedImageType
	}
}

// IsThumbnailReady returns true for files with ready thumbnails.
func (s *ThumbnailService) IsThumbnailReady(id rview.FileID) bool {
	s.inProgressTasksMu.RLock()
	_, inProgress := s.inProgressTasks[id]
	s.inProgressTasksMu.RUnlock()
	if inProgress {
		return true
	}

	return s.cache.Check(id) == nil
}

// OpenThumbnail returns io.ReadCloser for the image thumbnail. It waits for the files in queue, but no longer
// than context timeout.
func (s *ThumbnailService) OpenThumbnail(ctx context.Context, id rview.FileID) (io.ReadCloser, error) {
	isInProgress := func() (inProgress bool) {
		s.inProgressTasksMu.RLock()
		defer s.inProgressTasksMu.RUnlock()

		_, inProgress = s.inProgressTasks[id]
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

	return s.cache.Open(id)
}

// SendTask sends a task to the queue. It returns an error if the image format
// is not supported (it is detected by filepath). Filepath is passed to getImageFn, so it
// must be absolute.
//
// SendTask ignores duplicate tasks. However, it doesn't check files on disk.
func (s *ThumbnailService) SendTask(id rview.FileID, openFileFn rview.OpenFileFn) error {
	if s.stopped.Load() {
		return errors.New("can't send tasks after Shutdown call")
	}
	if !s.CanGenerateThumbnail(id) {
		return ErrUnsupportedImageFormat
	}

	var ignore bool
	func() {
		s.inProgressTasksMu.Lock()
		defer s.inProgressTasksMu.Unlock()

		if _, ok := s.inProgressTasks[id]; ok {
			ignore = true
			return
		}
		s.inProgressTasks[id] = struct{}{}
	}()
	if ignore {
		return nil
	}

	s.tasksCh <- generateThumbnailTask{
		FileID:     id,
		openFileFn: openFileFn,
	}
	return nil
}

func (s *ThumbnailService) GetMimeType(id rview.FileID) string {
	ext := convertToThumbnailFileID(id).GetExt()
	return mime.TypeByExtension(ext)
}

// Shutdown drops all tasks in the queue and waits for ones that are in progress
// with respect of the passed context.
func (s *ThumbnailService) Shutdown(ctx context.Context) error {
	s.stopped.Store(true)

	close(s.tasksCh)
	for range s.tasksCh { //nolint:revive
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.workersDoneCh:
		return nil
	}
}

// cacheWrapper converts all [rview.FileID] before passing them to [rview.Cache].
type cacheWrapper struct {
	c rview.Cache
}

func (c *cacheWrapper) Open(id rview.FileID) (io.ReadCloser, error) {
	return c.c.Open(convertToThumbnailFileID(id))
}

func (c *cacheWrapper) Check(id rview.FileID) error {
	return c.c.Check(convertToThumbnailFileID(id))
}

func (c *cacheWrapper) GetFilepath(id rview.FileID) (path string, err error) {
	return c.c.GetFilepath(convertToThumbnailFileID(id))
}

func (c *cacheWrapper) Write(id rview.FileID, r io.Reader) (err error) {
	return c.c.Write(convertToThumbnailFileID(id), r)
}

func (c *cacheWrapper) Remove(id rview.FileID) error {
	return c.c.Remove(convertToThumbnailFileID(id))
}

// convertToThumbnailFileID returns a file id for working with the thumbnail cache.
func convertToThumbnailFileID(id rview.FileID) rview.FileID {
	switch id.GetExt() {
	case ".heic":
		// We generate .jpeg thumbnails for .heic images.
		path := id.GetPath() + ".jpeg"
		return rview.NewFileID(path, id.GetModTime().Unix())

	default:
		return id
	}
}
