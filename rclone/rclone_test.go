package rclone

import (
	"context"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

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
		rclone := startRclone(t, rview.RcloneConfig{
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
		rclone := startRclone(t, rview.RcloneConfig{
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
		{Leaf: "Dark Souls/", IsDir: true, Size: 0, ModTime: 100},
		{Leaf: "Dark Souls 3/", IsDir: true, Size: 0, ModTime: 100},
		//
		{Leaf: "image.png", Size: 100, ModTime: 120},
		{Leaf: "1.txt", Size: 23, ModTime: 11},
		{Leaf: "2.txt", Size: 23, ModTime: 11},
		{Leaf: "12.txt", Size: 23, ModTime: 11},
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
		sortAndCheck(t, CompareDirEntryByName, []string{
			"arts/",
			"Dark Souls/",
			"Dark Souls 3/",
			"images/",
			"1.txt",
			"2.txt",
			"12.txt",
			"book copy.pdf",
			"book.pdf",
			"image.png",
		})
	})
	t.Run("size", func(t *testing.T) {
		sortAndCheck(t, compareDirEntryBySize, []string{
			"arts/",
			"Dark Souls/",
			"Dark Souls 3/",
			"images/",
			"1.txt",
			"2.txt",
			"12.txt",
			"image.png",
			"book copy.pdf",
			"book.pdf",
		})
	})
	t.Run("modtime", func(t *testing.T) {
		sortAndCheck(t, compareDirEntryByTime, []string{
			"1.txt",
			"2.txt",
			"12.txt",
			"Dark Souls/",
			"Dark Souls 3/",
			"image.png",
			"images/",
			"arts/",
			"book copy.pdf",
			"book.pdf",
		})
	})
}

func startRclone(t *testing.T, cfg rview.RcloneConfig) *Rclone {
	r := require.New(t)

	rclone, err := NewRclone(cfg)
	r.NoError(err)
	go func() {
		if err := rclone.Start(); err != nil {
			panic(err)
		}
	}()
	t.Cleanup(func() {
		err := rclone.Shutdown(context.Background())
		r.NoError(err)
	})

	for i := range 5 {
		_, err = rclone.GetAllFiles(t.Context())
		if err == nil {
			return rclone
		}
		time.Sleep(time.Duration(i) * 100 * time.Millisecond)
	}

	t.Fatal("rclone is not ready")
	return nil
}

func TestCompareStrings(t *testing.T) {
	check := func(t *testing.T, want []string) {
		in := slices.Clone(want)
		for range 10 {
			rand.Shuffle(len(in), func(i, j int) { in[i], in[j] = in[j], in[i] })
			slices.SortFunc(in, compareStrings)
			require.Equal(t, want, in)
		}
	}

	t.Run("ignore case", func(t *testing.T) {
		check(t, []string{"arts", "Dark"})
	})

	t.Run("natural order", func(t *testing.T) {
		check(t, []string{
			"2023-2024, 1",
			"2024, 2",
			"2024, 3",
			"2024, 4",
			"2024-2025, 1",
			"2025, 2",
			"test 1.txt",
			"test 2.txt",
			"test 10.txt",
			"test 15 10000.txt",
			"test 15 100000.txt",
			"test 123.txt",
			"test 123.txt",
			"test 150.txt",
			"test 1000 0001 1.txt",
			"test 1000 00001 2.txt",
			"w 12444",
			"w 012445",
			"w 012460",
			"y 123a2",
			"y 123b1",
		})
	})
}
