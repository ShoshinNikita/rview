package cache

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/ShoshinNikita/rview/pkg/metrics"
	"github.com/ShoshinNikita/rview/pkg/misc"
	"github.com/ShoshinNikita/rview/pkg/rlog"
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
	ticker := time.NewTimer(time.Minute)
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
	allFiles, err := c.loadAllFiles()
	if err != nil {
		logf := rlog.Errorf
		if errors.Is(err, fs.ErrNotExist) {
			logf = rlog.Warnf
		}
		metrics.CacheCleanerErrors.Inc()
		logf("couldn't load files to clean from cache %q: %s", c.cacheName, err)
		return
	}

	// Update metrics here because we can return early.
	var cacheSize int64
	for _, f := range allFiles {
		cacheSize += f.size
	}
	metrics.CacheSize.WithLabelValues(c.cacheName).Set(float64(cacheSize))

	filesToRemove := c.getFilesToRemove(allFiles)
	if len(filesToRemove) == 0 {
		rlog.Debugf("no files to remove from cache %q", c.cacheName)
		return
	}

	removedFiles, cleanedSpace, errs := c.removeFiles(filesToRemove)
	for _, err := range errs {
		metrics.CacheCleanerErrors.Inc()
		rlog.Error(err)
	}
	if removedFiles > 0 {
		rlog.Infof(
			"%d files have been removed from cache %q for a total of %s freed, got %d errors",
			removedFiles, c.cacheName, misc.FormatFileSize(cleanedSpace), len(errs),
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
