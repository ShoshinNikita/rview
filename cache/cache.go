package cache

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"

	"github.com/ShoshinNikita/rview/rview"
)

type DiskCache struct {
	dir string
}

func NewDiskCache(dir string) *DiskCache {
	return &DiskCache{
		dir: dir,
	}
}

// Open return an [io.ReadCloser] with cache content. If the file is not cached, it returns [rview.ErrCacheMiss].
func (c *DiskCache) Open(id rview.FileID) (io.ReadCloser, error) {
	path := c.generateFilepath(id)
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			err = rview.ErrCacheMiss
		}
		return nil, err
	}
	return file, nil
}

// Check can be used to check whether a file is cached. If the file is not cached, it returns [rview.ErrCacheMiss].
func (c *DiskCache) Check(id rview.FileID) error {
	path := c.generateFilepath(id)

	_, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			err = rview.ErrCacheMiss
		}
		return err
	}
	return nil
}

// GetSaveWriter returns a [io.WriteCloser] that must be used for writing cache content.
func (c *DiskCache) GetSaveWriter(id rview.FileID) (io.WriteCloser, error) {
	path := c.generateFilepath(id)

	dir := filepath.Dir(path)
	err := os.MkdirAll(dir, 0o777)
	if err != nil {
		return nil, fmt.Errorf("couldn't create dir %q: %w", dir, err)
	}

	file, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("couldn't create file %q: %w", path, err)
	}
	return file, nil
}

// generateFilepath generates a filepath of pattern '<dir>/<YYYY-MM>/<modTime>_<filename>'.
func (c *DiskCache) generateFilepath(id rview.FileID) string {
	modTime := id.GetModTime()
	subdir := modTime.Format("2006-01")
	res := strconv.Itoa(int(modTime.Unix())) + "_" + id.GetName()

	return filepath.Join(c.dir, subdir, res)
}
