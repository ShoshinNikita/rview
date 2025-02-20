package cache

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/ShoshinNikita/rview/rview"
	"github.com/stretchr/testify/require"
)

func TestDiskCache(t *testing.T) {
	t.Parallel()

	r := require.New(t)

	tempDir := t.TempDir()

	modTime := time.Date(2022, time.April, 15, 13, 5, 1, 0, time.UTC).Unix()
	fileID := rview.NewFileID("/home/Users/Персик/1.txt", modTime, 0)

	cache, err := NewDiskCache("", tempDir, Options{DisableCleaner: true})
	r.NoError(err)

	path := cache.generateFilepath(fileID)
	r.Equal(tempDir+"/2022-04/t1650027901_s0_4532f251c9f83c0ec83cc421f0a9a2b3.txt", path)

	t.Run("remove", func(t *testing.T) {
		r := require.New(t)

		r.False(checkFile(t, cache, fileID))

		err := cache.Write(fileID, strings.NewReader("hello world"))
		r.NoError(err)
		r.True(checkFile(t, cache, fileID))

		r.NoError(cache.Remove(fileID))
		r.False(checkFile(t, cache, fileID))
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

func TestDiskCache_FilesWithSameName(t *testing.T) {
	t.Parallel()

	r := require.New(t)

	modTime := time.Date(2023, time.April, 14, 0, 0, 0, 0, time.UTC).Unix()
	file1 := rview.NewFileID("/qwerty/1.txt", modTime, 0)
	file2 := rview.NewFileID("/abcdef/1.txt", modTime, 0)

	cache, err := NewDiskCache("", t.TempDir(), Options{DisableCleaner: true})
	r.NoError(err)

	err = cache.Write(file1, strings.NewReader("hello world"))
	r.NoError(err)

	r.False(checkFile(t, cache, file2))
}

func checkFile(t *testing.T, cache *DiskCache, id rview.FileID) bool {
	rc, err := cache.Open(id)
	if errors.Is(err, ErrCacheMiss) {
		return false
	}
	require.NoError(t, err)
	rc.Close()
	return true
}
