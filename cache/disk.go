package cache

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"

	"github.com/ShoshinNikita/rview/metrics"
	"github.com/ShoshinNikita/rview/rlog"
	"github.com/ShoshinNikita/rview/rview"
)

type DiskCache struct {
	absDir string
}

var _ rview.Cache = (*DiskCache)(nil)

func NewDiskCache(dir string) (*DiskCache, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("couldn't get absolute path: %w", err)
	}
	return &DiskCache{
		absDir: absDir,
	}, nil
}

// Open return an [io.ReadCloser] with cache content. If the file is not cached, it returns [rview.ErrCacheMiss].
func (c *DiskCache) Open(id rview.FileID) (io.ReadCloser, error) {
	path := c.generateFilepath(id)

	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			metrics.CacheMisses.Inc()
			return nil, rview.ErrCacheMiss
		}

		metrics.CacheErrors.Inc()
		return nil, err
	}

	metrics.CacheHits.Inc()
	return file, nil
}

// Check can be used to check whether a file is cached. If the file is not cached, it returns [rview.ErrCacheMiss].
func (c *DiskCache) Check(id rview.FileID) error {
	path := c.generateFilepath(id)

	_, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			metrics.CacheMisses.Inc()
			return rview.ErrCacheMiss
		}

		metrics.CacheErrors.Inc()
		return err
	}

	metrics.CacheHits.Inc()
	return nil
}

// GetSaveWriter returns a [io.WriteCloser] that must be used for writing cache content.
// Caller must remove a cache file in case of any error by calling the "remove" function.
func (c *DiskCache) GetSaveWriter(id rview.FileID) (_ io.WriteCloser, remove func(), err error) {
	path := c.generateFilepath(id)

	dir := filepath.Dir(path)
	err = os.MkdirAll(dir, 0o777)
	if err != nil {
		return nil, nil, fmt.Errorf("couldn't create dir %q: %w", dir, err)
	}

	file, err := os.Create(path)
	if err != nil {
		return nil, nil, fmt.Errorf("couldn't create file %q: %w", path, err)
	}

	remove = func() {
		if err := os.Remove(path); err != nil {
			rlog.Errorf("couldn't remove cache file %q via callback after error: %s", path, err)
		}
	}

	return file, remove, nil
}

// generateFilepath generates a filepath of pattern '<dir>/<YYYY-MM>/<modTime>_<filename>'.
func (c *DiskCache) generateFilepath(id rview.FileID) string {
	modTime := id.GetModTime()
	subdir := modTime.Format("2006-01")
	res := strconv.Itoa(int(modTime.Unix())) + "_" + id.GetName()

	return filepath.Join(c.absDir, subdir, res)
}
