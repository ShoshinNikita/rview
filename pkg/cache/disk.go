package cache

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/ShoshinNikita/rview/pkg/metrics"
	"github.com/ShoshinNikita/rview/rview"
)

var ErrCacheMiss = errors.New("cache miss")

type DiskCache struct {
	absDir  string
	cleaner *Cleaner
}

type Options struct {
	DisableCleaner bool
	MaxSize        int64
}

func NewDiskCache(absDir string, opts Options) (cache *DiskCache, err error) {
	if !filepath.IsAbs(absDir) {
		return nil, fmt.Errorf("dir should be absolute")
	}

	cache = &DiskCache{
		absDir: absDir,
	}
	if !opts.DisableCleaner {
		name := filepath.Base(absDir)
		cache.cleaner, err = NewCleaner(name, absDir, opts.MaxSize)
		if err != nil {
			return nil, fmt.Errorf("couldn't prepare cache cleaner: %w", err)
		}
	}
	return cache, nil
}

// Open return an [io.ReadCloser] with cache content. If the file is not cached, it returns [rview.ErrCacheMiss].
func (c *DiskCache) Open(id rview.FileID) (io.ReadCloser, error) {
	path := c.generateFilepath(id)

	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			metrics.CacheMisses.Inc()
			return nil, ErrCacheMiss
		}

		metrics.CacheErrors.Inc()
		return nil, err
	}

	metrics.CacheHits.Inc()
	return file, nil
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

// Write copies the content of the passed [io.Reader] to the cache file associated with [rview.FileID].
func (c *DiskCache) Write(id rview.FileID, r io.Reader) error {
	filepath, err := c.GetFilepath(id)
	if err != nil {
		return fmt.Errorf("couldn't get filepath: %w", err)
	}

	f, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("couldn't create file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, r); err != nil {
		return fmt.Errorf("couldn't write file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("couldn't close file: %w", err)
	}
	return nil
}

// Remove removes the cache file associated with passed [rview.FileID]. To remove
// cache files over time use [Cleaner], cache files should be manually removed only
// in case of an error.
func (c *DiskCache) Remove(id rview.FileID) error {
	return os.Remove(c.generateFilepath(id))
}

// generateFilepath generates a filepath of pattern '<dir>/<YYYY-MM>/t<mod time>_s<size>_<hashed filepath>.<ext>'.
func (c *DiskCache) generateFilepath(id rview.FileID) string {
	modTime := id.GetModTime()
	subdir := modTime.Format("2006-01")

	hash := md5.Sum([]byte(id.GetPath()))
	name := hex.EncodeToString(hash[:])
	filename := fmt.Sprintf("t%d_s%d_%s", modTime.Unix(), id.GetSize(), name+id.GetExt())

	return filepath.Join(c.absDir, subdir, filename)
}

func (c *DiskCache) Shutdown(ctx context.Context) error {
	if c.cleaner != nil {
		return c.cleaner.Shutdown(ctx)
	}
	return nil
}
