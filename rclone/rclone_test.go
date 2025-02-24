package rclone

import (
	"context"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/ShoshinNikita/rview/pkg/cache"
	"github.com/ShoshinNikita/rview/rview"
	"github.com/stretchr/testify/require"
)

func TestRclone_DirCache(t *testing.T) {
	r := require.New(t)

	getEntries := func(t *testing.T, rclone *Rclone) []string {
		info, err := rclone.GetDirInfo(t.Context(), "/", "", "")
		r.NoError(err)

		res := make([]string, 0, len(info.Entries))
		for _, e := range info.Entries {
			if e.Leaf != "" {
				res = append(res, e.Leaf)
			}
		}
		return res
	}

	t.Run("cache disabled", func(t *testing.T) {
		dir := t.TempDir()
		rclone := startRclone(t, cache.NewInMemoryCache(), rview.RcloneConfig{
			Target:      dir,
			Port:        32142,
			DirCacheTTL: 0,
		})

		err := os.WriteFile(filepath.Join(dir, "1.txt"), nil, 0600)
		r.NoError(err)
		r.Equal([]string{"1.txt"}, getEntries(t, rclone))

		err = os.WriteFile(filepath.Join(dir, "2.txt"), nil, 0600)
		r.NoError(err)
		r.Equal([]string{"1.txt", "2.txt"}, getEntries(t, rclone))
	})

	t.Run("cache enabled", func(t *testing.T) {
		dir := t.TempDir()
		rclone := startRclone(t, cache.NewInMemoryCache(), rview.RcloneConfig{
			Target:      dir,
			Port:        32142,
			DirCacheTTL: time.Hour,
		})

		err := os.WriteFile(filepath.Join(dir, "1.txt"), nil, 0600)
		r.NoError(err)
		r.Equal([]string{"1.txt"}, getEntries(t, rclone))

		err = os.WriteFile(filepath.Join(dir, "2.txt"), nil, 0600)
		r.NoError(err)
		r.Equal([]string{"1.txt"}, getEntries(t, rclone)) // got data from cache

		// Expire cache.
		{
			item := rclone.dirCache.Get("/")
			item.expiresAt = time.Now().Add(-time.Hour)
		}

		err = os.WriteFile(filepath.Join(dir, "2.txt"), nil, 0600)
		r.NoError(err)
		r.Equal([]string{"1.txt", "2.txt"}, getEntries(t, rclone)) // got data from rclone
	})
}

func TestRclone_SortEntries(t *testing.T) {
	entries := []DirEntry{
		{Leaf: "images/", IsDir: true, Size: 0, ModTime: 123},
		{Leaf: "arts/", IsDir: true, Size: 0, ModTime: 321},
		//
		{Leaf: "image.png", Size: 100, ModTime: 120},
		{Leaf: "1.txt", Size: 23, ModTime: 11},
		{Leaf: "book.pdf", Size: 234, ModTime: 400},
		{Leaf: "book copy.pdf", Size: 234, ModTime: 400},
	}

	sortAndCheck := func(t *testing.T, sortFn func(a, b DirEntry) int, want []string) {
		entries := slices.Clone(entries)

		for range 10 {
			rand.Shuffle(len(entries), func(i, j int) { entries[i], entries[j] = entries[j], entries[i] })
			slices.SortFunc(entries, sortFn)

			var filenames []string
			for _, e := range entries {
				filenames = append(filenames, e.Leaf)
			}
			require.Equal(t, want, filenames)
		}
	}

	t.Run("name", func(t *testing.T) {
		sortAndCheck(t, sortByName, []string{
			"arts/",
			"images/",
			"1.txt",
			"book copy.pdf",
			"book.pdf",
			"image.png",
		})
	})
	t.Run("size", func(t *testing.T) {
		sortAndCheck(t, sortBySize, []string{
			"arts/",
			"images/",
			"1.txt",
			"image.png",
			"book copy.pdf",
			"book.pdf",
		})
	})
	t.Run("modtime", func(t *testing.T) {
		sortAndCheck(t, sortByTime, []string{
			"1.txt",
			"image.png",
			"images/",
			"arts/",
			"book copy.pdf",
			"book.pdf",
		})
	})
}

func TestRclone_OpenFile(t *testing.T) {
	r := require.New(t)

	dir := t.TempDir()

	// Prepare files.
	modtime := time.Date(2025, 2, 20, 0, 0, 0, 0, time.UTC)
	file := filepath.Join(dir, "1.txt")
	fileID := rview.NewFileID("1.txt", modtime.Unix(), 11)
	{
		err := os.WriteFile(file, []byte("hello world"), 0600)
		r.NoError(err)
		err = os.Chtimes(file, modtime, modtime)
		r.NoError(err)
	}

	cache := &cacheStub{
		inMemory: cache.NewInMemoryCache(),
	}
	rclone := startRclone(t, cache, rview.RcloneConfig{
		Target: dir,
		Port:   37144,
	})

	getFile := func() (string, error) {
		rc, err := rclone.OpenFile(t.Context(), fileID)
		if err != nil {
			return "", err
		}
		defer rc.Close()

		data, err := io.ReadAll(rc)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}

	// Parallel requests - only 1 write to the cache.
	var (
		wg    sync.WaitGroup
		resCh = make(chan struct {
			data string
			err  error
		}, 100)
	)
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()

			data, err := getFile()
			resCh <- struct {
				data string
				err  error
			}{data, err}
		}()
	}
	wg.Wait()
	close(resCh)
	for res := range resCh {
		r.NoError(res.err)
		r.Equal("hello world", res.data)
	}
	r.Equal(1, cache.writeCount)
	r.Equal(21, cache.openCount)

	// File should be served from the cache.
	err := os.Remove(file)
	r.NoError(err)
	data, err := getFile()
	r.NoError(err)
	r.Equal("hello world", data)

	// No file in the cache.
	err = cache.inMemory.Remove(fileID)
	r.NoError(err)
	_, err = getFile()
	r.Error(err)
	r.Contains(err.Error(), "status code: 404")
}

func startRclone(t *testing.T, cache Cache, cfg rview.RcloneConfig) *Rclone {
	r := require.New(t)

	rclone, err := NewRclone(cache, cfg)
	r.NoError(err)
	go func() {
		if err := rclone.Start(); err != nil {
			panic(err)
		}
	}()
	t.Cleanup(func() {
		err := rclone.Shutdown(context.Background()) //nolint:usetesting
		r.NoError(err)
	})

	for i := range 5 {
		_, _, err = rclone.GetAllFiles(t.Context())
		if err == nil {
			return rclone
		}
		time.Sleep(time.Duration(i) * 100 * time.Millisecond)
	}

	t.Fatal("rclone is not ready")
	return nil
}

type cacheStub struct {
	inMemory   *cache.InMemoryCache
	openCount  int
	writeCount int
}

func (c *cacheStub) Open(id rview.FileID) (io.ReadCloser, error) {
	c.openCount++
	return c.inMemory.Open(id)
}

func (c *cacheStub) Write(id rview.FileID, r io.Reader) error {
	c.writeCount++
	return c.inMemory.Write(id, r)
}
