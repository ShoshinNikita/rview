package cache

import (
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ShoshinNikita/rview/rview"
	"github.com/stretchr/testify/require"
)

func TestDiskCache(t *testing.T) {
	t.Parallel()

	r := require.New(t)

	modTime := time.Date(2022, time.April, 15, 13, 5, 1, 0, time.UTC).Unix()
	fileID := rview.NewFileID("/home/users/1.txt", modTime)

	cache, err := NewDiskCache(os.TempDir())
	r.NoError(err)

	path := cache.generateFilepath(fileID)
	r.Equal(os.TempDir()+"/2022-04/1650027901_1.txt", path)

	t.Cleanup(func() {
		os.Remove(path)
	})

	t.Run("check", func(t *testing.T) {
		r := require.New(t)

		err := cache.Check(fileID)
		r.ErrorIs(err, rview.ErrCacheMiss)

		_, err = cache.Open(fileID)
		r.ErrorIs(err, rview.ErrCacheMiss)
	})

	t.Run("remove", func(t *testing.T) {
		r := require.New(t)

		err = cache.Check(fileID)
		r.ErrorIs(err, rview.ErrCacheMiss)

		err := cache.Write(fileID, strings.NewReader("hello world"))
		r.NoError(err)

		err = cache.Check(fileID)
		r.NoError(err)

		r.NoError(cache.Remove(fileID))
		r.ErrorIs(cache.Check(fileID), rview.ErrCacheMiss)
	})

	t.Run("read", func(t *testing.T) {
		r := require.New(t)

		err := cache.Write(fileID, strings.NewReader("hello world"))
		r.NoError(err)

		rc, err := cache.Open(fileID)
		r.NoError(err)

		data, err := io.ReadAll(rc)
		r.NoError(err)
		r.Equal("hello world", string(data))

		r.NoError(rc.Close())
	})
}
