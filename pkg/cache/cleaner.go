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

	"github.com/ShoshinNikita/rview/pkg/misc"
	"github.com/ShoshinNikita/rview/pkg/rlog"
)

type NoopCleaner struct{}

func NewNoopCleaner() *NoopCleaner {
	return &NoopCleaner{}
}

func (NoopCleaner) Shutdown(context.Context) error {
	return nil
}

// Cleaner controls the total size of the cache.
type Cleaner struct {
	dir              string
	cleanupInterval  time.Duration
	maxTotalFileSize int64 // in bytes

	stopCh                 chan struct{}
	cleanupProcessFinished chan struct{}
}

type fileInfo struct {
	path    string
	modTime time.Time
	size    int64
}

func NewCleaner(dir string, maxTotalFileSize int64) *Cleaner {
	c := &Cleaner{
		dir:              dir,
		cleanupInterval:  5 * time.Minute,
		maxTotalFileSize: maxTotalFileSize,
		//
		stopCh:                 make(chan struct{}),
		cleanupProcessFinished: make(chan struct{}),
	}

	go c.startCleanupProcess()

	return c
}

func (c Cleaner) startCleanupProcess() {
	ticker := time.NewTimer(c.cleanupInterval)
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

func (c Cleaner) cleanup() {
	allFiles, err := c.loadAllFiles()
	if err != nil {
		logf := rlog.Errorf
		if errors.Is(err, fs.ErrNotExist) {
			logf = rlog.Warnf
		}
		logf("couldn't load files to clean: %s", err)
		return
	}

	filesToRemove := c.getFilesToRemove(allFiles)
	if len(filesToRemove) == 0 {
		rlog.Debug("no files to remove from cache")
		return
	}

	removedFiles, cleanedSpace, errs := c.removeFiles(filesToRemove)
	for _, err := range errs {
		rlog.Error(err)
	}
	if removedFiles > 0 {
		rlog.Infof(
			"%d files have been removed from cache for a total of %s freed, got %d errors",
			removedFiles, misc.FormatFileSize(cleanedSpace), len(errs),
		)
	}
}

func (c Cleaner) loadAllFiles() (files []fileInfo, err error) {
	err = filepath.Walk(c.dir, func(path string, info fs.FileInfo, err error) error {
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
