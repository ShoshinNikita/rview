package rclone

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ShoshinNikita/rview/pkg/cache"
	"github.com/ShoshinNikita/rview/rview"
	"github.com/stretchr/testify/require"
)

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
	rclone := startRclone(t, dir, cache)

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

func startRclone(t *testing.T, dir string, cache Cache) *Rclone {
	r := require.New(t)

	rclone, err := NewRclone(cache, rview.RcloneConfig{
		Target: dir,
		Port:   37511,
	})
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
