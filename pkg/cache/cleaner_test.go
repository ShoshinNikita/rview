package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCleaner_loadAllFilesAndRemove(t *testing.T) {
	t.Parallel()

	r := require.New(t)

	dir := t.TempDir()

	for path, size := range map[string]int{
		"a/1.txt":                   100,
		"a/2.txt":                   300,
		"a/b/33.txt":                1024,
		"5.txt":                     15,
		"test/test/test/qwerty.txt": 111,
	} {
		path = filepath.Join(dir, path)
		dir := filepath.Dir(path)
		err := os.MkdirAll(dir, 0o700)
		r.NoError(err)

		err = os.WriteFile(path, make([]byte, size), 0600)
		r.NoError(err)
	}

	c := Cleaner{absDir: dir}

	// Check all files.
	{
		files, err := c.loadAllFiles()
		r.NoError(err)

		for i := range files {
			files[i].modTime = time.Time{}
		}
		r.ElementsMatch(
			[]fileInfo{
				{path: filepath.Join(dir, "a/1.txt"), size: 100},
				{path: filepath.Join(dir, "a/2.txt"), size: 300},
				{path: filepath.Join(dir, "a/b/33.txt"), size: 1024},
				{path: filepath.Join(dir, "5.txt"), size: 15},
				{path: filepath.Join(dir, "test/test/test/qwerty.txt"), size: 111},
			},
			files,
		)
	}

	// Remove some files.
	{
		files := []fileInfo{
			{path: filepath.Join(dir, "a/1.txt"), size: 100},
			{path: filepath.Join(dir, "a/2.txt"), size: 300},
			{path: filepath.Join(dir, "a/b/33.txt"), size: 1024},
		}

		removedFiles, cleanedSpace, errs := c.removeFiles(files)
		if len(errs) != 0 {
			t.Fatalf("got errors: %v", errs)
		}
		r.Equal(3, removedFiles)
		r.Equal(1424, int(cleanedSpace))
	}

	// Check left files.
	{
		files, err := c.loadAllFiles()
		r.NoError(err)

		for i := range files {
			files[i].modTime = time.Time{}
		}
		r.Equal(
			[]fileInfo{
				{path: filepath.Join(dir, "5.txt"), size: 15},
				{path: filepath.Join(dir, "test/test/test/qwerty.txt"), size: 111},
			},
			files,
		)
	}
}

func TestCleaner_getFilesToRemove(t *testing.T) {
	t.Parallel()

	newTime := func(day int, hour int) time.Time {
		return time.Date(2022, time.October, day, hour, 0, 0, 0, time.UTC)
	}

	tests := []struct {
		name             string
		maxTotalFileSize int64
		files            []fileInfo
		//
		wantFilenames []string
	}{
		{
			name:             "nothing to remove",
			maxTotalFileSize: 1 << 10, // 1 KiB
			files: []fileInfo{
				{path: "1", modTime: newTime(17, 0), size: 1 << 7}, // 128 B
				{path: "2", modTime: newTime(17, 0), size: 1 << 8}, // 256 B
				{path: "3", modTime: newTime(17, 0), size: 1 << 9}, // 512 B
			},
			wantFilenames: nil,
		},
		{
			name:             "remove all files",
			maxTotalFileSize: 1 << 10, // 1 KiB
			files: []fileInfo{
				{path: "1", modTime: newTime(17, 0), size: 1 << 20},
				{path: "2", modTime: newTime(17, 0), size: 1 << 20},
				{path: "3", modTime: newTime(17, 0), size: 1 << 20},
				{path: "4", modTime: newTime(17, 0), size: 1 << 20},
			},
			wantFilenames: []string{"1", "2", "3", "4"},
		},
		{
			name:             "remove some files",
			maxTotalFileSize: 5 << 20, // 5 MiB
			files: []fileInfo{
				{path: "4", modTime: newTime(11, 0), size: 1 << 19},         // 0.5 MiB
				{path: "5", modTime: newTime(13, 0), size: 1 << 19},         // 0.5 MiB
				{path: "6", modTime: newTime(14, 0), size: 1<<20 + 256<<10}, // 1.2 MiB
				{path: "7", modTime: newTime(15, 0), size: 1<<20 + 512<<10}, // 1.5 MiB
				{path: "8", modTime: newTime(15, 0), size: 1 << 20},         // 1 MiB
				{path: "9", modTime: newTime(16, 0), size: 3 << 20},         // 3 MiB
			},
			wantFilenames: []string{"4", "5", "6", "7"},
		},
	}
	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			if tt.maxTotalFileSize == 0 {
				t.Fatalf("zero max total file size")
			}

			c := Cleaner{
				maxTotalFileSize: tt.maxTotalFileSize,
			}
			got := c.getFilesToRemove(tt.files)
			var gotPaths []string
			for _, f := range got {
				gotPaths = append(gotPaths, f.path)
			}
			require.ElementsMatch(t, tt.wantFilenames, gotPaths)
		})
	}
}
