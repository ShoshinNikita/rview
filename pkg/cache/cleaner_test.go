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
	wantFiles := []fileInfo{
		{path: "a/1.txt", size: 100},
		{path: "a/2.txt", size: 300},
		{path: "a/b/33.txt", size: 1024},
		{path: "5.txt", size: 15},
		{path: "test/test/test/qwerty.txt", size: 111},
	}
	for i := range wantFiles {
		wantFiles[i].path = filepath.Join(dir, wantFiles[i].path)

		file := wantFiles[i]
		dir := filepath.Dir(file.path)
		err := os.MkdirAll(dir, 0o777)
		r.NoError(err)

		f, err := os.Create(file.path)
		r.NoError(err)

		_, err = f.Write(make([]byte, file.size))
		r.NoError(err)

		err = f.Close()
		r.NoError(err)
	}

	c := Cleaner{dir: dir}

	gotFiles, err := c.loadAllFiles()
	r.NoError(err)

	for i := range gotFiles {
		gotFiles[i].modTime = time.Time{}
		r.Contains(gotFiles[i].path, dir)
	}
	r.ElementsMatch(wantFiles, gotFiles)

	removedFiles, cleanedSpace, errs := c.removeFiles(wantFiles[:3])
	if len(errs) != 0 {
		t.Fatalf("got errors: %v", errs)
	}
	r.Equal(3, removedFiles)
	r.Equal(1424, int(cleanedSpace))

	gotFilesAfterRemove, err := c.loadAllFiles()
	r.NoError(err)

	for i := range gotFilesAfterRemove {
		gotFilesAfterRemove[i].modTime = time.Time{}
	}
	r.ElementsMatch(
		wantFiles[3:],
		gotFilesAfterRemove,
	)
}

func TestCleaner_getFilesToRemove(t *testing.T) {
	t.Parallel()

	newTime := func(day int, hour int) time.Time {
		return time.Date(2022, time.October, day, hour, 0, 0, 0, time.UTC)
	}

	tests := []struct {
		name             string
		maxFileAge       time.Duration
		maxTotalFileSize int64
		now              time.Time
		files            []fileInfo
		//
		wantFilenames []string
	}{
		{
			name:             "all files are old",
			maxFileAge:       24 * time.Hour, // 1 day
			maxTotalFileSize: 1 << 10,        // 1 KiB
			now:              newTime(18, 0),
			files: []fileInfo{
				{path: "10", modTime: newTime(1, 0), size: 1 << 20},
				{path: "20", modTime: newTime(2, 0), size: 1 << 20},
				{path: "30", modTime: newTime(3, 0), size: 1 << 20},
				{path: "40", modTime: newTime(4, 0), size: 1 << 20},
			},
			wantFilenames: []string{"10", "20", "30", "40"},
		},
		{
			name:             "remove all files because of size limit",
			maxFileAge:       7 * 24 * time.Hour, // 7 days
			maxTotalFileSize: 1 << 10,            // 1 KiB
			now:              newTime(18, 0),
			files: []fileInfo{
				{path: "1", modTime: newTime(17, 0), size: 1 << 20},
				{path: "2", modTime: newTime(17, 0), size: 1 << 20},
				{path: "3", modTime: newTime(17, 0), size: 1 << 20},
				{path: "4", modTime: newTime(17, 0), size: 1 << 20},
			},
			wantFilenames: []string{"1", "2", "3", "4"},
		},
		{
			name:             "mixed",
			maxFileAge:       7 * 24 * time.Hour, // 7 days
			maxTotalFileSize: 5 << 20,            // 5 MiB
			now:              newTime(18, 0),
			files: []fileInfo{
				// Old files
				{path: "1", modTime: newTime(1, 37)},
				{path: "3", modTime: newTime(4, 51)},
				{path: "2", modTime: newTime(10, 0)},
				// New files (3.7 MiB)
				{path: "4", modTime: newTime(11, 0), size: 1 << 19},         // 0.5 MiB
				{path: "5", modTime: newTime(13, 0), size: 1 << 19},         // 0.5 MiB
				{path: "6", modTime: newTime(14, 0), size: 1<<20 + 256<<10}, // 1.2 MiB
				{path: "7", modTime: newTime(15, 0), size: 1<<20 + 512<<10}, // 1.5 MiB
				// New files (4 MiB)
				{path: "8", modTime: newTime(15, 0), size: 1 << 20}, // 1 MiB
				{path: "9", modTime: newTime(16, 0), size: 3 << 20}, // 3 MiB
			},
			wantFilenames: []string{"1", "2", "3", "4", "5", "6", "7"},
		},
	}
	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			if tt.maxFileAge == 0 {
				t.Fatalf("zero max file age")
			}
			if tt.maxTotalFileSize == 0 {
				t.Fatalf("zero max total file size")
			}
			if tt.now.IsZero() {
				t.Fatalf("zero now")
			}

			c := Cleaner{
				maxFileAge:       tt.maxFileAge,
				maxTotalFileSize: tt.maxTotalFileSize,
			}
			got := c.getFilesToRemove(tt.files, tt.now)
			var gotPaths []string
			for _, f := range got {
				gotPaths = append(gotPaths, f.path)
			}
			require.ElementsMatch(t, tt.wantFilenames, gotPaths)
		})
	}
}
