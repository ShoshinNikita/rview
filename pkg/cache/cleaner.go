package cache

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/ShoshinNikita/rview/pkg/metrics"
	"github.com/ShoshinNikita/rview/pkg/misc"
)

// Cleaner controls the total size of the cache.
type Cleaner struct {
	cacheName        string
	absDir           string
	maxTotalFileSize int64 // in bytes

	stopCh                 chan struct{}
	cleanupProcessFinished chan struct{}
}

type fileInfo struct {
	path    string
	modTime time.Time
	size    int64
}

func NewCleaner(cacheName, absDir string, maxTotalFileSize int64) (*Cleaner, error) {
	if !filepath.IsAbs(absDir) {
		return nil, fmt.Errorf("dir should be absolute")
	}

	c := &Cleaner{
		cacheName:        cacheName,
		absDir:           absDir,
		maxTotalFileSize: maxTotalFileSize,
		//
		stopCh:                 make(chan struct{}),
		cleanupProcessFinished: make(chan struct{}),
	}

	go c.startCleanupProcess()

	return c, nil
}

func (c *Cleaner) startCleanupProcess() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		// Run immediately.
		c.cleanup()

		select {
		case <-ticker.C:
			continue
		case <-c.stopCh:
			close(c.cleanupProcessFinished)
			return
		}
	}
}

func (c *Cleaner) cleanup() {
	slog := slog.With("cache", c.cacheName)

	allFiles, err := c.loadAllFiles()
	if err != nil {
		logFn := slog.Error
		if errors.Is(err, fs.ErrNotExist) {
			logFn = slog.Warn
		}
		metrics.CacheCleanerErrors.Inc()
		logFn("couldn't load files to clean from cache", "err", err)
		return
	}

	// Update metrics here because we can return early.
	var cacheSize int64
	for _, f := range allFiles {
		cacheSize += f.size
	}
	metrics.CacheSize(c.cacheName).Set(float64(cacheSize))

	filesToRemove := c.getFilesToRemove(allFiles)
	if len(filesToRemove) == 0 {
		slog.Debug("no files to remove from cache")
		return
	}

	removedFiles, cleanedSpace, errs := c.removeFiles(filesToRemove)
	for _, err := range errs {
		metrics.CacheCleanerErrors.Inc()
		slog.Error("cleanup error", "err", err)
	}
	if removedFiles > 0 {
		slog.Info(
			"cleanup finished", "removed_files", removedFiles,
			"freed_space", misc.FormatSize(cleanedSpace),
		)
	}
}

func (c *Cleaner) loadAllFiles() (files []fileInfo, err error) {
	err = filepath.Walk(c.absDir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		files = append(files, fileInfo{
			path:    path,
			modTime: info.ModTime(),
			size:    info.Size(),
		})
		return nil
	})
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// Directory hasn't been created yet.
			return nil, nil
		}
		return nil, err
	}
	return files, nil
}

func (c Cleaner) getFilesToRemove(files []fileInfo) []fileInfo {
	var totalSize int64
	for _, file := range files {
		totalSize += file.size
	}
	if totalSize < c.maxTotalFileSize {
		return nil
	}

	// Remove old files first.
	slices.SortFunc(files, func(a, b fileInfo) int {
		return a.modTime.Compare(b.modTime)
	})

	var index int
	for i, file := range files {
		totalSize -= file.size
		if totalSize < c.maxTotalFileSize {
			// Other files satisfy the size limit.
			index = i + 1
			break
		}
	}
	if index == 0 {
		// Remove all files.
		index = len(files)
	}

	return files[:index]
}

func (c Cleaner) removeFiles(files []fileInfo) (removedFiles int, cleanedSpace int64, errs []error) {
	for _, file := range files {
		err := os.Remove(file.path)
		if err != nil {
			errs = append(errs, fmt.Errorf("couldn't remove file %q from cache: %w", file.path, err))
			continue
		}
		removedFiles++
		cleanedSpace += file.size
	}
	return removedFiles, cleanedSpace, errs
}

func (c Cleaner) Shutdown(ctx context.Context) error {
	close(c.stopCh)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.cleanupProcessFinished:
		return nil
	}
}
