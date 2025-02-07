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
	cache    rview.Cache
	rclone   Rclone
	resizeFn func(originalFile, cacheFile string, id ThumbnailID, size rview.ThumbnailSize) error
	// useOriginalImageThresholdSize defines the maximum size of an original image that should be
	// used without resizing. The main purpose of resizing is to reduce image size, and with small
	// files it is not always possible - after resizing they become just larger.
	useOriginalImageThresholdSize int64
	thumbnailsFormat              rview.ThumbnailsFormat

	workersCount int

	tasksCh           chan generateThumbnailTask
	inProgressTasks   map[ThumbnailID]struct{}
	inProgressTasksMu sync.Mutex

	stopped       *atomic.Bool
	workersDoneCh chan struct{}
}

type Rclone interface {
	OpenFile(context.Context, rview.FileID) (io.ReadCloser, error)
}

type ThumbnailID struct {
	rview.FileID
}

type generateThumbnailTask struct {
	fileID      rview.FileID
	thumbnailID ThumbnailID
	useOriginal bool
	size        rview.ThumbnailSize
}

func CheckVips() error {
	cmd := exec.Command("vips", "--version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("vips is not installed: %w", err)
	}
	return nil
}

// NewThumbnailService prepares a new service for thumbnail generation.
// It generates thumbnails only for large files and uses the small ones as-is.
//
// For some images we can generate thumbnails of different formats. For example,
// for .heic images we generate .jpeg thumbnails.
func NewThumbnailService(
	rclone Rclone, cache rview.Cache, workersCount int, thumbnailsFormat rview.ThumbnailsFormat,
) *ThumbnailService {

	r := &ThumbnailService{
		rclone:                        rclone,
		cache:                         cache,
		resizeFn:                      resizeWithVips,
		useOriginalImageThresholdSize: 200 << 10, // 200 KiB
		thumbnailsFormat:              thumbnailsFormat,
		//
		workersCount: workersCount,
		//
		tasksCh:         make(chan generateThumbnailTask, 10_000),
		inProgressTasks: make(map[ThumbnailID]struct{}),
		//
		stopped:       new(atomic.Bool),
		workersDoneCh: make(chan struct{}),
	}

	go r.startWorkers()

	return r
}

func (s *ThumbnailService) GenerateThumbnailsForSmallFiles() {
	s.useOriginalImageThresholdSize = 0
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
	rc, err := s.rclone.OpenFile(ctx, task.fileID)
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

	if task.useOriginal {
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

	err = s.resizeFn(tempFile.Name(), cacheFilepath, task.thumbnailID, task.size)
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
func resizeWithVips(originalFile, cacheFile string, id ThumbnailID, thumbnailSize rview.ThumbnailSize) error {
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

	size := "1024>"
	if thumbnailSize == rview.ThumbnailLarge {
		size = "2048>"
	}

	cmd := exec.Command(
		"vipsthumbnail",
		"--rotate", // auto-rotate
		originalFile,
		"--size", size,
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

// OpenThumbnail returns [io.ReadCloser] for the image thumbnail. It generates a new thumbnail if needed.
// Only the first call to OpenThumbnail generates a thumbnail.
func (s *ThumbnailService) OpenThumbnail(
	ctx context.Context, id rview.FileID, size rview.ThumbnailSize,
) (rc io.ReadCloser, contentType string, err error) {

	if getImageType(id) == unsupportedImageType {
		return nil, "", fmt.Errorf("%w: %q", ErrUnsupportedImageFormat, id.GetExt())
	}

	thumbnailID := ThumbnailID{FileID: id}

	useOriginal := s.shouldUseOriginalImage(id)
	if !useOriginal {
		thumbnailID, err = s.newThumbnailID(id, size)
		if err != nil {
			return nil, "", fmt.Errorf("couldn't get thumbnail id: %w", err)
		}
	}

	contentType = mime.TypeByExtension(thumbnailID.GetExt())

	if s.cache.Check(thumbnailID.FileID) == nil {
		// Thumbnail already exists.
		rc, err := s.cache.Open(thumbnailID.FileID)
		return rc, contentType, err
	}

	isInProgress := func(addTask bool) bool {
		s.inProgressTasksMu.Lock()
		defer s.inProgressTasksMu.Unlock()

		if _, ok := s.inProgressTasks[thumbnailID]; ok {
			return true
		}
		if addTask {
			s.inProgressTasks[thumbnailID] = struct{}{}
		}
		return false
	}
	if !isInProgress(true) {
		s.tasksCh <- generateThumbnailTask{
			fileID:      id,
			thumbnailID: thumbnailID,
			useOriginal: useOriginal,
			size:        size,
		}
	}

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		// Check immediately
		if !isInProgress(false) {
			break
		}

		select {
		case <-ticker.C:
			continue
		case <-ctx.Done():
			return nil, "", ctx.Err()
		}
	}

	rc, err = s.cache.Open(thumbnailID.FileID)
	return rc, contentType, err
}

func (s *ThumbnailService) shouldUseOriginalImage(id rview.FileID) bool {
	switch getImageType(id) {
	case gifImageType:
		// vipsthumbnail can't resize gifs: https://github.com/libvips/libvips/issues/61#issuecomment-168169916
		return true

	case heicImageType:
		// Always generate thumbnails for .heic because most browsers don't support it.
		return false
	}

	return id.GetSize() < s.useOriginalImageThresholdSize
}

// newThumbnailID converts [rview.FileID] to [ThumbnailID].
func (s *ThumbnailService) newThumbnailID(id rview.FileID, size rview.ThumbnailSize) (ThumbnailID, error) {
	path := id.GetPath()

	newExt, err := s.getThumbnailExt(id)
	if err != nil {
		return ThumbnailID{}, err
	}

	suffix := ".thumbnail"
	if size == rview.ThumbnailLarge {
		suffix += "-large"
	}
	if newExt == "" {
		originalExt := pkgPath.Ext(path)
		path = strings.TrimSuffix(path, originalExt)
		path = path + suffix + originalExt
	} else {
		path = path + suffix + newExt

	}

	return ThumbnailID{
		FileID: rview.NewFileID(path, id.GetModTime().Unix(), id.GetSize()),
	}, nil
}

func (s *ThumbnailService) getThumbnailExt(id rview.FileID) (string, error) {
	imageType := getImageType(id)

	var newExt string
	switch s.thumbnailsFormat {
	case rview.JpegThumbnails:
		switch imageType {
		case jpegImageType: // already .jpeg
			newExt = ""
		case pngImageType:
			newExt = ".jpeg"
		case gifImageType: // we can't generate thumbnail for .gif, but we can save the original file
			newExt = ""
		case webpImageType: // already efficient enough and supported by modern browsers
			newExt = ""
		case heicImageType: // most browsers don't support .heic
			newExt = ".jpeg"
		case avifImageType: // already efficient enough and supported by modern browsers
			newExt = ""
		default:
			return "", fmt.Errorf("%w: %q", ErrUnsupportedImageFormat, id.GetExt())
		}

	case rview.AvifThumbnails:
		switch imageType {
		case jpegImageType:
			newExt = ".avif"
		case pngImageType:
			newExt = ".avif"
		case gifImageType: // we can't generate thumbnail for .gif, but we can save the original file
			newExt = ""
		case webpImageType: // already efficient enough and supported by modern browsers
			newExt = ""
		case heicImageType: // most browsers don't support .heic
			newExt = ".avif"
		case avifImageType: // already .avif
			newExt = ""
		default:
			return "", fmt.Errorf("%w: %q", ErrUnsupportedImageFormat, id.GetExt())
		}

	default:
		return "", fmt.Errorf("invalid thumbnails format: %q", s.thumbnailsFormat)
	}
	return newExt, nil
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
