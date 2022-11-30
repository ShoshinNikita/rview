package cache

import (
	"io"
	"os"
	"testing"
	"time"

	"github.com/ShoshinNikita/rview/rview"
	"github.com/ShoshinNikita/rview/util/testutil"
)

func TestDiskCache(t *testing.T) {
	t.Parallel()

	modTime := time.Date(2022, time.April, 15, 13, 5, 1, 0, time.UTC).Unix()
	fileID := rview.NewFileID("/home/users/1.txt", modTime)

	cache, err := NewDiskCache(os.TempDir())
	testutil.NoError(t, err)

	path := cache.generateFilepath(fileID)
	testutil.Equal(t, os.TempDir()+"/2022-04/1650027901_1.txt", path)

	t.Cleanup(func() {
		os.Remove(path)
	})

	t.Run("check", func(t *testing.T) {
		err := cache.Check(fileID)
		testutil.IsError(t, err, rview.ErrCacheMiss)

		_, err = cache.Open(fileID)
		testutil.IsError(t, err, rview.ErrCacheMiss)
	})

	t.Run("remove", func(t *testing.T) {
		path, err := cache.GetFilepath(fileID)
		testutil.NoError(t, err)

		err = cache.Check(fileID)
		testutil.IsError(t, err, rview.ErrCacheMiss)

		err = os.WriteFile(path, []byte("hello world"), 0o600)
		testutil.NoError(t, err)

		err = cache.Check(fileID)
		testutil.NoError(t, err)

		testutil.NoError(t, cache.Remove(fileID))
		testutil.IsError(t, cache.Check(fileID), rview.ErrCacheMiss)
	})

	t.Run("read", func(t *testing.T) {
		path, err := cache.GetFilepath(fileID)
		testutil.NoError(t, err)

		err = os.WriteFile(path, []byte("hello world"), 0o600)
		testutil.NoError(t, err)

		rc, err := cache.Open(fileID)
		testutil.NoError(t, err)

		data, err := io.ReadAll(rc)
		testutil.NoError(t, err)
		testutil.Equal(t, "hello world", string(data))

		testutil.NoError(t, rc.Close())
	})
}
