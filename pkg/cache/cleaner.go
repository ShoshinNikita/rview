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

	"github.com/ShoshinNikita/rview/pkg/rlog"
)

type NoopCleaner struct{}

func NewNoopCleaner() *NoopCleaner {
	return &NoopCleaner{}
}

func (NoopCleaner) Shutdown(context.Context) error {
	return nil
}

// Cleaner can be used remove old files and control the total size of cache.
type Cleaner struct {
	dir              string
	cleanupInterval  time.Duration
	maxFileAge       time.Duration
	maxTotalFileSize int64 // in bytes

	stopCh                 chan struct{}
	cleanupProcessFinished chan struct{}
}

type fileInfo struct {
	path    string
	modTime time.Time
	size    int64
}

func NewCleaner(dir string, maxFileAge time.Duration, maxTotalFileSize int64) *Cleaner {
	c := &Cleaner{
		dir:              dir,
		cleanupInterval:  time.Hour,
		maxFileAge:       maxFileAge,
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
		c.cleanup(time.Now())

		select {
		case <-ticker.C:
			continue
		case <-c.stopCh:
			close(c.cleanupProcessFinished)
			return
		}
	}
}

func (c Cleaner) cleanup(now time.Time) {
	rlog.Debugf("start cleanup")

	allFiles, err := c.loadAllFiles()
	if err != nil {
		logf := rlog.Errorf
		if errors.Is(err, fs.ErrNotExist) {
			logf = rlog.Warnf
		}
		logf("couldn't load files to clean: %s", err)
		return
	}

	filesToRemove := c.getFilesToRemove(allFiles, now)
	if len(filesToRemove) == 0 {
		return
	}

	rlog.Debugf("should remove %d cached files", len(filesToRemove))

	errs := c.removeFiles(filesToRemove)
	for _, err := range errs {
		rlog.Error(err)
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

func (c Cleaner) getFilesToRemove(files []fileInfo, now time.Time) []fileInfo {
	minModTime := now.Add(-c.maxFileAge)

	var (
		oldFiles             []fileInfo
		activeFiles          []fileInfo
		activeFilesTotalSize int64
	)
	for _, file := range files {
		if file.modTime.Before(minModTime) {
			oldFiles = append(oldFiles, file)
		} else {
			activeFiles = append(activeFiles, file)
			activeFilesTotalSize += file.size
		}
	}
	if activeFilesTotalSize < c.maxTotalFileSize {
		// Should remove only old files.
		return oldFiles
	}

	// Remove old files first.
	slices.SortFunc(activeFiles, func(a, b fileInfo) int {
		return a.modTime.Compare(b.modTime)
	})

	var index int
	for i, file := range activeFiles {
		activeFilesTotalSize -= file.size
		if activeFilesTotalSize < c.maxTotalFileSize {
			// Other files satisfy the size limit.
			index = i + 1
			break
		}
	}
	if index == 0 {
		// Impossible, just in case, remove all files.
		index = len(activeFiles)
	}

	return append(oldFiles, activeFiles[:index]...)
}

func (c Cleaner) removeFiles(files []fileInfo) (errs []error) {
	for _, file := range files {
		err := os.Remove(file.path)
		if err != nil {
			errs = append(errs, fmt.Errorf("couldn't remove cached file %q: %w", file.path, err))
		}
	}
	return errs
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
