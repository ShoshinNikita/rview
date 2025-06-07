package thumbnails

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"os/exec"
	pkgPath "path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ShoshinNikita/rview/pkg/metrics"
	"github.com/ShoshinNikita/rview/pkg/rlog"
	"github.com/ShoshinNikita/rview/rview"
	"github.com/prometheus/client_golang/prometheus"
)

func init() {
	for _, size := range []ThumbnailSize{ThumbnailSmall, ThumbnailMedium, ThumbnailLarge} {
		metrics.ThumbnailsSizeRatio.WithLabelValues(string(size))
	}
}

type ThumbnailSize string

const (
	ThumbnailSmall  ThumbnailSize = "small"
	ThumbnailMedium ThumbnailSize = "medium"
	ThumbnailLarge  ThumbnailSize = "large"
)

var ErrUnsupportedImageFormat = errors.New("unsupported image format")

type ThumbnailService struct {
	cache              Cache
	originalImageCache Cache
	openImageLocks     *sync.Map

	rclone   Rclone
	resizeFn func(originalFile, cacheFile string, id ThumbnailID, size ThumbnailSize) error
	// useOriginalImageThresholdSize defines the maximum size of an original image that should be
	// used without resizing. The main purpose of resizing is to reduce image size, and with small
	// files it is not always possible - after resizing they become just larger.
	useOriginalImageThresholdSize int64
	thumbnailsFormat              rview.ThumbnailsFormat
	processRawImages              bool

	workersCount int

	tasksCh           chan generateThumbnailTask
	inProgressTasks   map[ThumbnailID]struct{}
	inProgressTasksMu sync.Mutex

	stopped       *atomic.Bool
	workersDoneCh chan struct{}
}

type Cache interface {
	Open(id rview.FileID) (io.ReadCloser, error)
	GetFilepath(id rview.FileID) (path string, err error)
	Write(id rview.FileID, r io.Reader) (err error)
	Remove(id rview.FileID) error
}

type Rclone interface {
	OpenFile(context.Context, rview.FileID) (io.ReadCloser, error)
	RequestFileRange(ctx context.Context, id rview.FileID, rangeStart, rangeEnd int) (io.ReadCloser, error)
}

type ThumbnailID struct {
	rview.FileID
}

type generateThumbnailTask struct {
	fileID      rview.FileID
	thumbnailID ThumbnailID
	useOriginal bool
	size        ThumbnailSize
}

func CheckDeps() error {
	if err := exec.Command("vips", "--version").Run(); err != nil {
		return fmt.Errorf("vips is not installed: %w", err)
	}
	if err := exec.Command("exiftool", "-ver").Run(); err != nil {
		return fmt.Errorf("exiftool is not installed: %w", err)
	}
	return nil
}

// NewThumbnailService prepares a new service for thumbnail generation.
// It generates thumbnails only for large files and uses the small ones as-is.
//
// For some images we can generate thumbnails of different formats. For example,
// for .heic images we generate .jpeg thumbnails.
func NewThumbnailService(
	rclone Rclone, cache, originalImageCache Cache, workersCount int, thumbnailsFormat rview.ThumbnailsFormat,
	processRawImages bool,
) *ThumbnailService {

	r := &ThumbnailService{
		cache:              cache,
		originalImageCache: originalImageCache,
		openImageLocks:     new(sync.Map),
		//
		rclone:                        rclone,
		resizeFn:                      resizeWithVips,
		useOriginalImageThresholdSize: 200 << 10, // 200 KiB
		thumbnailsFormat:              thumbnailsFormat,
		processRawImages:              processRawImages,
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
					metrics.ThumbnailsSizeRatio.
						WithLabelValues(string(task.size)).
						Observe(float64(stats.originalSize) / float64(stats.thumbnailSize))

					msg := fmt.Sprintf(
						"%s thumbnail for %q was generated in %s, original size: %s, new size: %s",
						task.size, task.fileID.GetPath(), dur, toMiB(stats.originalSize), toMiB(stats.thumbnailSize),
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
	cacheFilepath, err := s.cache.GetFilepath(task.thumbnailID.FileID)
	if err != nil {
		return stats{}, fmt.Errorf("couldn't get path for a thumbnail file: %w", err)
	}

	downloadImageTimer := prometheus.NewTimer(metrics.ThumbnailsDownloadImageDuration)
	rc, err := s.openImage(ctx, task.fileID)
	if err != nil {
		return stats{}, fmt.Errorf("couldn't get image reader: %w", err)
	}
	defer rc.Close()

	if task.useOriginal {
		size := task.fileID.GetSize()
		err := createCacheFileFromReader(rc, cacheFilepath, size)
		if err != nil {
			return stats{}, err
		}
		downloadImageTimer.ObserveDuration()

		return stats{
			originalSize:      size,
			thumbnailSize:     size,
			originalImageUsed: true,
		}, nil
	}

	tempFile, err := os.CreateTemp("", "rview-*")
	if err != nil {
		return stats{}, fmt.Errorf("couldn't create temp image file: %w", err)
	}
	defer func() {
		_ = tempFile.Close()

		if err := os.Remove(tempFile.Name()); err != nil {
			rlog.Errorf("couldn't remove temp image file: %s", err)
		}
	}()

	originalSize, err := io.Copy(tempFile, rc)
	if err != nil {
		return stats{}, fmt.Errorf("couldn't load image: %w", err)
	}
	if getImageType(task.fileID) != rawImageType && originalSize != task.fileID.GetSize() {
		return stats{}, fmt.Errorf("temp file has wrong size, expected: %d, got: %d", task.fileID.GetSize(), originalSize)
	}
	if err := tempFile.Close(); err != nil {
		return stats{}, fmt.Errorf("couldn't close temp file: %w", err)
	}
	downloadImageTimer.ObserveDuration()

	resizeTimer := prometheus.NewTimer(metrics.ThumbnailsResizeDuration)
	err = s.resizeFn(tempFile.Name(), cacheFilepath, task.thumbnailID, task.size)
	if err != nil {
		if err := s.cache.Remove(task.thumbnailID.FileID); err != nil {
			rlog.Warnf("couldn't remove thumbnail for %s after resize error: %s", task.fileID, err)
		}

		return stats{}, err
	}
	resizeTimer.ObserveDuration()

	info, err := os.Stat(cacheFilepath)
	if err != nil {
		return stats{}, fmt.Errorf("couldn't get stats of a cache file: %w", err)
	}
	return stats{
		originalSize:  originalSize,
		thumbnailSize: info.Size(),
	}, nil
}

func (s *ThumbnailService) openImage(ctx context.Context, id rview.FileID) (rc io.ReadCloser, err error) {
	// Don't allow parallel openImage calls with the same file id because we will be saving
	// the file content to the cache.
	mu, _ := s.openImageLocks.LoadOrStore(id, new(sync.Mutex))
	mu.(*sync.Mutex).Lock()
	defer mu.(*sync.Mutex).Unlock()

	rc, err = s.originalImageCache.Open(id)
	if err == nil {
		metrics.ThumbnailsOriginalImagesUsedFromCache.Inc()
		return rc, nil
	}

	if getImageType(id) == rawImageType {
		rc, err = s.extractPreviewFromRawImage(ctx, id)
	} else {
		rc, err = s.rclone.OpenFile(ctx, id)
	}
	if err != nil {
		return nil, err
	}
	err = s.originalImageCache.Write(id, rc)
	if err != nil {
		return nil, fmt.Errorf("couldn't write original image to the cache: %w", err)
	}
	return s.originalImageCache.Open(id)
}

func (s *ThumbnailService) extractPreviewFromRawImage(ctx context.Context, id rview.FileID) (io.ReadCloser, error) {
	decodeJSON := func(output []byte) (ExifToolJsonResult, error) {
		var resp []ExifToolJsonResult
		if err := json.Unmarshal(output, &resp); err != nil {
			return ExifToolJsonResult{}, fmt.Errorf("couldn't decode exiftool output: %w", err)
		}
		if len(resp) != 1 {
			return ExifToolJsonResult{}, fmt.Errorf("wrong number of items in exiftool output: expected 1, got %d", len(resp))
		}
		return resp[0], nil
	}

	var (
		jpgFromRaw  io.ReadCloser
		orientation string
	)
	switch ext := id.GetExt(); ext {
	case ".arw":
		// All necessary information can be found at the beginning of a file.
		rc, err := s.rclone.RequestFileRange(ctx, id, 0, 4096)
		if err != nil {
			return nil, fmt.Errorf("couldn't request file headers: %w", err)
		}
		defer rc.Close()

		cmd := exec.Command("exiftool", "-json", "-PreviewImageStart", "-PreviewImageLength", "-Orientation", "-")
		cmd.Stdin = rc
		output, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("exiftool failed: %w", err)
		}
		res, err := decodeJSON(output)
		if err != nil {
			return nil, err
		}
		orientation = res.Orientation

		offset := res.PreviewImageStart
		length := res.PreviewImageLength
		if offset == nil || length == nil {
			return nil, fmt.Errorf("offset and/or length are missing")
		}
		jpgFromRaw, err = s.rclone.RequestFileRange(ctx, id, *offset, *offset+*length)
		if err != nil {
			return nil, fmt.Errorf("couldn't request jpeg preview: %w", err)
		}

	case ".rw2":
		// All necessary information can be found at the beginning of a file.
		rc, err := s.rclone.RequestFileRange(ctx, id, 0, 4096)
		if err != nil {
			return nil, fmt.Errorf("couldn't request file headers: %w", err)
		}
		defer rc.Close()

		// Size and offset of JpgFromRaw can be found only in the html dump. Use -htmlDump0 for the absolute offsets.
		cmd := exec.Command("exiftool", "-htmlDump0", "-")
		cmd.Stdin = rc
		output, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("exiftool failed: %w", err)
		}
		data := regexp.MustCompile(`>JpgFromRaw<.*?Size: (\d+) bytes.*?Value offset: 0x([0-9a-f]+)`).FindSubmatch(output)
		if len(data) == 0 {
			return nil, fmt.Errorf("couldn't extract 'Size' and 'Value offset' from htmldump")
		}
		length, err := strconv.Atoi(string(data[1]))
		if err != nil {
			return nil, fmt.Errorf("couldn't parse 'Size': %w", err)
		}
		offset, err := strconv.ParseInt(string(data[2]), 16, 64)
		if err != nil {
			return nil, fmt.Errorf("couldn't parse 'Value offset': %w", err)
		}
		jpgFromRaw, err = s.rclone.RequestFileRange(ctx, id, int(offset), int(offset)+length)
		if err != nil {
			return nil, fmt.Errorf("couldn't request jpeg preview: %w", err)
		}

	case ".nef", ".cr3":
		// .NEF (Nikon) and .CR3 (Canon) RAW files contain jpeg previews in the middle of a file.
		// So, we have to send the entire file to exiftool.
		rc, err := s.rclone.OpenFile(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("couldn't open file %q: %w", id, err)
		}

		cmd := exec.Command("exiftool", "-b", "-json", "-Orientation", "-JpgFromRaw", "-")
		cmd.Stdin = rc

		output, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("exiftool failed: %w", err)
		}
		res, err := decodeJSON(output)
		if err != nil {
			return nil, err
		}

		orientation = res.Orientation
		jpgFromRaw = io.NopCloser(bytes.NewReader(res.JpgFromRaw))

	default:
		return nil, fmt.Errorf("unsupported RAW image format: %q", ext)
	}

	if orientation == "" || strings.Contains(orientation, "Horizontal") {
		return jpgFromRaw, nil
	}

	// Have to add 'Orientation' tag to jpeg.
	r, w := io.Pipe()
	go func() {
		defer jpgFromRaw.Close()

		cmd := exec.Command("exiftool", "-Orientation="+orientation, "-") //nolint:gosec
		cmd.Stdin = jpgFromRaw
		cmd.Stdout = w
		err := cmd.Run()
		if err != nil {
			err = fmt.Errorf("couldn't add orientation tag to jpeg: %w", err)
		}
		w.CloseWithError(err)
	}()
	return r, nil
}

//nolint:tagliatelle
type ExifToolJsonResult struct {
	PreviewImageStart  *int           `json:"PreviewImageStart"`
	PreviewImageLength *int           `json:"PreviewImageLength"`
	Orientation        string         `json:"Orientation"`
	JpgFromRaw         ExifToolBase64 `json:"JpgFromRaw"`
}

type ExifToolBase64 []byte

func (b *ExifToolBase64) UnmarshalJSON(data []byte) error {
	if !bytes.HasPrefix(data, []byte(`"base64:`)) {
		return errors.New(`expected prefix "base64:"`)
	}

	// Replace `"base64:` with `"`.
	data = data[7:]
	data[0] = '"'

	var res []byte
	if err := json.Unmarshal(data, &res); err != nil {
		return err
	}
	*b = ExifToolBase64(res)
	return nil
}

func createCacheFileFromReader(r io.Reader, cacheFilepath string, originalSize int64) error {
	cacheFile, err := os.Create(cacheFilepath)
	if err != nil {
		return fmt.Errorf("couldn't create cache file: %w", err)
	}

	copied, err := io.Copy(cacheFile, r)
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
func resizeWithVips(originalFile, cacheFile string, id ThumbnailID, thumbnailSize ThumbnailSize) error {
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

	var size string
	switch thumbnailSize {
	case ThumbnailSmall:
		size = "256>"
	case ThumbnailMedium:
		size = "1024>"
	case ThumbnailLarge:
		size = "2048>"
	default:
		return fmt.Errorf("invalid thumbnail size: %q", thumbnailSize)
	}

	//nolint:gosec
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
func (s *ThumbnailService) CanGenerateThumbnail(id rview.FileID) bool {
	switch getImageType(id) {
	case unsupportedImageType:
		return false
	case rawImageType:
		return s.processRawImages
	default:
		return true
	}
}

type imageType int

const (
	unsupportedImageType imageType = iota
	jpegImageType
	pngImageType
	gifImageType
	webpImageType
	heicImageType
	avifImageType
	rawImageType = 7
)

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
	case ".arw", ".rw2", ".nef", ".cr3":
		return rawImageType
	default:
		return unsupportedImageType
	}
}

// OpenThumbnail returns [io.ReadCloser] for the image thumbnail. It generates a new thumbnail if needed.
// Only the first call to OpenThumbnail generates a thumbnail.
func (s *ThumbnailService) OpenThumbnail(
	ctx context.Context, id rview.FileID, size ThumbnailSize,
) (rc io.ReadCloser, contentType string, err error) {

	if s.stopped.Load() {
		return nil, "", errors.New("service was stopped")
	}

	if getImageType(id) == unsupportedImageType {
		return nil, "", fmt.Errorf("%w: %q", ErrUnsupportedImageFormat, id.GetExt())
	}

	if size == "" {
		size = ThumbnailMedium
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

	if rc, err := s.cache.Open(thumbnailID.FileID); err == nil {
		// Thumbnail already exists.
		return rc, contentType, nil
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
		if !isInProgress(false) { //nolint:staticcheck
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

	case rawImageType:
		// Always extract jpeg preview from RAW files.
		return false
	}

	return id.GetSize() < s.useOriginalImageThresholdSize
}

// newThumbnailID converts [rview.FileID] to [ThumbnailID].
func (s *ThumbnailService) newThumbnailID(id rview.FileID, size ThumbnailSize) (ThumbnailID, error) {
	path := id.GetPath()

	newExt, err := s.getThumbnailExt(id)
	if err != nil {
		return ThumbnailID{}, err
	}

	suffix := ".thumbnail-" + string(size)
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
		case rawImageType:
			newExt = ".jpeg"
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
		case rawImageType:
			newExt = ".avif"
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
	for range s.tasksCh {
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.workersDoneCh:
		return nil
	}
}
