package cache

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"

	"github.com/ShoshinNikita/rview/pkg/metrics"
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

// GetFilepath returns the absolute path of the cache file associated with passed [rview.FileID].
// It creates all directories, so the caller can create the cache file without any additional
// actions.
func (c *DiskCache) GetFilepath(id rview.FileID) (path string, err error) {
	path = c.generateFilepath(id)

	dir := filepath.Dir(path)
	err = os.MkdirAll(dir, 0o777)
	if err != nil {
		return "", fmt.Errorf("couldn't create dir %q: %w", dir, err)
	}

	return path, nil
}

// Remove removes the cache file associated with passed [rview.FileID]. To remove
// cache files over time use [Cleaner], cache files should be manually removed only
// in case of an error.
func (c *DiskCache) Remove(id rview.FileID) error {
	return os.Remove(c.generateFilepath(id))
}

// generateFilepath generates a filepath of pattern '<dir>/<YYYY-MM>/<modTime>_<filename>'.
func (c *DiskCache) generateFilepath(id rview.FileID) string {
	modTime := id.GetModTime()
	subdir := modTime.Format("2006-01")
	res := strconv.Itoa(int(modTime.Unix())) + "_" + id.GetName()

	return filepath.Join(c.absDir, subdir, res)
}
