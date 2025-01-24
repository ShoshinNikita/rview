package thumbnails

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	pkgPath "path"
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
	webpImageType
	heicImageType
	avifImageType
)

var (
	ErrUnsupportedImageFormat = errors.New("unsupported image format")
)

type ThumbnailService struct {
	cache      rview.Cache
	openFileFn rview.OpenFileFn
	resizeFn   func(originalFile, cacheFile string, id rview.ThumbnailID) error
	// useOriginalImageThresholdSize defines the maximum size of an original image that should be
	// used without resizing. The main purpose of resizing is to reduce image size, and with small
	// files it is not always possible - after resizing they become just larger.
	useOriginalImageThresholdSize int64

	workersCount int

	tasksCh           chan generateThumbnailTask
	inProgressTasks   map[rview.ThumbnailID]struct{}
	inProgressTasksMu sync.RWMutex

	stopped       *atomic.Bool
	workersDoneCh chan struct{}
}

type generateThumbnailTask struct {
	fileID      rview.FileID
	thumbnailID rview.ThumbnailID
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
func NewThumbnailService(openFileFn rview.OpenFileFn, cache rview.Cache, workersCount int, generateThumbnailsForSmallFiles bool) *ThumbnailService {
	r := &ThumbnailService{
		openFileFn:                    openFileFn,
		cache:                         cache,
		resizeFn:                      resizeWithVips,
		useOriginalImageThresholdSize: 200 << 10, // 200 KiB
		//
		workersCount: workersCount,
		//
		tasksCh:         make(chan generateThumbnailTask, 10_000),
		inProgressTasks: make(map[rview.ThumbnailID]struct{}),
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
	for range s.workersCount {
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
					rlog.Errorf("couldn't process task for %q: %s", task.fileID.GetPath(), err)

				case stats.originalImageUsed:
					metrics.ThumbnailsOriginalImageUsed.Inc()
					rlog.Debugf("use original image for %q, size: %s", task.fileID.GetPath(), toMiB(stats.originalSize))

				default:
					metrics.ThumbnailsProcessTaskDuration.Observe(dur.Seconds())
					metrics.ThumbnailsSizeRatio.Observe(float64(stats.originalSize) / float64(stats.thumbnailSize))

					msg := fmt.Sprintf(
						"thumbnail for %q was generated in %s, original size: %s, new size: %s",
						task.fileID.GetPath(), dur, toMiB(stats.originalSize), toMiB(stats.thumbnailSize),
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
				delete(s.inProgressTasks, task.thumbnailID)
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
	rc, err := s.openFileFn(ctx, task.fileID)
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

	cacheFilepath, err := s.cache.GetFilepath(task.thumbnailID.FileID)
	if err != nil {
		return stats{}, fmt.Errorf("couldn't get path of a cache file: %w", err)
	}

	imageType := getImageType(task.fileID)

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

	err = s.resizeFn(tempFile.Name(), cacheFilepath, task.thumbnailID)
	if err != nil {
		if err := s.cache.Remove(task.thumbnailID.FileID); err != nil {
			rlog.Errorf("couldn't remove thumbnail for %s after resize error: %s", task.fileID, err)
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
func resizeWithVips(originalFile, cacheFile string, id rview.ThumbnailID) error {
	output := cacheFile
	switch t := getImageType(id.FileID); t {
	// Ignore .heic, .png and etc. because thumbnail id must already have the correct extension.
	case jpegImageType:
		output += "[Q=80,optimize_coding,keep=icc]"
	case webpImageType:
		output += "[keep=icc]"
	case avifImageType:
		// 'Q=65' provides decent image quality (similar to jpeg's 'Q=80') and small
		// file sizes - ~22% less than the default 'Q=75'.
		//
		// 'speed=8' is ~72% faster than the default 'speed=5', and the image quality is good enough.
		// The file size is consistent across different 'speed' values - ±3%.
		output += "[Q=65,speed=8,keep=icc]"
	default:
		return fmt.Errorf("unsupported thumbnail format: %q", t)
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
		rlog.Infof("vips stderr for %q: %q", id, stderr.String())
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
	case ".avif":
		return avifImageType
	default:
		return unsupportedImageType
	}
}

// NewThumbnailID converts [rview.FileID] to [rview.ThumbnailID]. The caller should first
// check [ThumbnailService.CanGenerateThumbnail] - NewThumbnailID panics if the file id
// can't be transformed to thumbnail id.
func (*ThumbnailService) NewThumbnailID(id rview.FileID) rview.ThumbnailID {
	path := id.GetPath()
	originalExt := pkgPath.Ext(path)
	path = strings.TrimSuffix(path, originalExt)

	newExt, ok := map[imageType]string{
		// .jpeg thumbnails are much smaller.
		pngImageType: ".jpeg",
		// .heic - most browsers don't support it.
		heicImageType: ".jpeg",
		// Already .jpeg.
		jpegImageType: "",
		// We can't generate thumbnail for .gif, but we can save the original file.
		gifImageType: "",
		// These formats are already efficient enough and supported by modern browsers.
		webpImageType: "",
		avifImageType: "",
	}[getImageType(id)]
	if !ok {
		// Just in case.
		panic(fmt.Errorf("%w: %q", ErrUnsupportedImageFormat, id.GetExt()))
	}

	// Add .thumbnail to be able to more easily distinguish thumbnails from original files.
	path += ".thumbnail" + originalExt + newExt

	return rview.ThumbnailID{
		FileID: rview.NewFileID(path, id.GetModTime().Unix()),
	}
}

// IsThumbnailReady returns true for files with ready thumbnails.
func (s *ThumbnailService) IsThumbnailReady(id rview.ThumbnailID) bool {
	s.inProgressTasksMu.RLock()
	_, inProgress := s.inProgressTasks[id]
	s.inProgressTasksMu.RUnlock()
	if inProgress {
		return true
	}

	return s.cache.Check(id.FileID) == nil
}

// OpenThumbnail returns io.ReadCloser for the image thumbnail. It waits for the files in queue, but no longer
// than context timeout.
func (s *ThumbnailService) OpenThumbnail(ctx context.Context, id rview.ThumbnailID) (io.ReadCloser, error) {
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

	return s.cache.Open(id.FileID)
}

// SendTask sends a task to the queue. It returns an error if the image format
// is not supported (it is detected by filepath). Filepath is passed to getImageFn, so it
// must be absolute.
//
// SendTask ignores duplicate tasks. However, it doesn't check files on disk.
func (s *ThumbnailService) SendTask(id rview.FileID) error {
	if s.stopped.Load() {
		return errors.New("can't send tasks after Shutdown call")
	}
	if !s.CanGenerateThumbnail(id) {
		return ErrUnsupportedImageFormat
	}

	thumbnailID := s.NewThumbnailID(id)

	var ignore bool
	func() {
		s.inProgressTasksMu.Lock()
		defer s.inProgressTasksMu.Unlock()

		if _, ok := s.inProgressTasks[thumbnailID]; ok {
			ignore = true
			return
		}
		s.inProgressTasks[thumbnailID] = struct{}{}
	}()
	if ignore {
		return nil
	}

	s.tasksCh <- generateThumbnailTask{
		fileID:      id,
		thumbnailID: thumbnailID,
	}
	return nil
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
