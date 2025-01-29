package tests

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ShoshinNikita/rview/cmd"
	"github.com/ShoshinNikita/rview/rview"
	"github.com/ShoshinNikita/rview/web"
	"github.com/stretchr/testify/require"
)

var (
	rviewAPIAddr string

	testRview     *cmd.Rview
	testRviewDone <-chan struct{}
	testRviewOnce sync.Once
)

// startTestRview starts a global test rview instance via sync.Once. This function
// doesn't accept *testing.T because we want to panic in case of an error instead
// of calling t.Fatal.
func startTestRview() {
	testRviewOnce.Do(func() {
		tempDir, err := os.MkdirTemp("", "rview-tests-*")
		if err != nil {
			panic(fmt.Errorf("couldn't create temp dir: %w", err))
		}

		cfg := rview.Config{
			ServerPort: mustGetFreePort(),
			Dir:        tempDir,
			//
			Rclone: rview.RcloneConfig{
				Target: "./testdata",
				Port:   mustGetFreePort(),
			},
			//
			ImagePreviewMode:       rview.ImagePreviewModeThumbnails,
			ThumbnailsFormat:       rview.JpegThumbnails,
			ThumbnailsWorkersCount: 1,
		}
		rviewAPIAddr = fmt.Sprintf("http://localhost:%d", cfg.ServerPort)
		rcloneAddr := fmt.Sprintf("http://localhost:%d", cfg.Rclone.Port)

		testRview = cmd.NewRview(cfg)
		if err := testRview.Prepare(); err != nil {
			panic(fmt.Errorf("couldn't prepare rview: %w", err))
		}

		testRviewDone = testRview.Start(func() {
			panic(fmt.Errorf("rview error"))
		})

		// Wait for rclone to start.
		for i := range 10 {
			if i != 0 {
				time.Sleep(20 * time.Millisecond)
			}

			resp, err := http.DefaultClient.Get(rcloneAddr) //nolint:noctx
			if err == nil && resp.StatusCode == 200 {
				break
			}
		}

		// Wait for components to be ready.
		for i := range 10 {
			if i != 0 {
				time.Sleep(100 * time.Millisecond)
			}

			resp, err := http.DefaultClient.Get(rviewAPIAddr + "/api/search?search=test") //nolint:noctx
			if err == nil && resp.StatusCode == 200 {
				break
			}
		}
	})
}

func mustGetFreePort() int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(fmt.Errorf("couldn't get free port: %w", err))
	}
	defer l.Close()

	addr := l.Addr().String()

	index := strings.LastIndex(addr, ":")
	port, err := strconv.Atoi(addr[index+1:])
	if err != nil {
		panic(fmt.Errorf("couldn't parse port: %w", err))
	}

	return port
}

func TestAPI_GetDirInfo(t *testing.T) {
	startTestRview()

	t.Run("check full info", func(t *testing.T) {
		r := require.New(t)

		dirInfo := getDirInfo(t, "/", "")
		r.Equal(
			web.DirInfo{
				Dir: "/",
				Breadcrumbs: []web.DirBreadcrumb{
					{Link: "/ui/", Text: "Home"},
				},
				Entries: []web.DirEntry{
					{
						Filename:             "Audio",
						IsDir:                true,
						ModTime:              mustParseTime(t, "2022-08-09 00:15:30"),
						HumanReadableModTime: "2022-08-09 00:15:30 UTC",
						DirURL:               "/api/dir/Audio/",
						WebDirURL:            "/ui/Audio/",
						IconName:             "folder",
					},
					{
						Filename:             "Images",
						IsDir:                true,
						ModTime:              mustParseTime(t, "2023-01-01 18:35:00"),
						HumanReadableModTime: "2023-01-01 18:35:00 UTC",
						DirURL:               "/api/dir/Images/",
						WebDirURL:            "/ui/Images/",
						IconName:             "folder",
					},
					{
						Filename:             "Other",
						IsDir:                true,
						ModTime:              mustParseTime(t, "2022-09-08 11:37:02"),
						HumanReadableModTime: "2022-09-08 11:37:02 UTC",
						DirURL:               "/api/dir/Other/",
						WebDirURL:            "/ui/Other/",
						IconName:             "folder",
					},
					{
						Filename:             "Video",
						IsDir:                true,
						ModTime:              mustParseTime(t, "2022-09-08 11:37:02"),
						HumanReadableModTime: "2022-09-08 11:37:02 UTC",
						DirURL:               "/api/dir/Video/",
						WebDirURL:            "/ui/Video/",
						IconName:             "folder",
					},
					{
						Filename:             "archive.7z",
						Size:                 0,
						HumanReadableSize:    "0 B",
						ModTime:              mustParseTime(t, "2022-04-07 05:23:55"),
						HumanReadableModTime: "2022-04-07 05:23:55 UTC",
						FileType:             rview.FileTypeUnknown,
						CanPreview:           false,
						OriginalFileURL:      "/api/file/archive.7z?mod_time=1649309035",
						IconName:             "zip",
					},
					{
						Filename:             "Lorem ipsum.txt",
						Size:                 943,
						HumanReadableSize:    "943 B",
						ModTime:              mustParseTime(t, "2023-02-27 15:00:00"),
						HumanReadableModTime: "2023-02-27 15:00:00 UTC",
						FileType:             rview.FileTypeText,
						CanPreview:           true,
						OriginalFileURL:      "/api/file/Lorem%20ipsum.txt?mod_time=1677510000",
						IconName:             "document",
					},
					{
						Filename:             "main.go",
						Size:                 73,
						HumanReadableSize:    "73 B",
						ModTime:              mustParseTime(t, "2022-04-07 18:23:55"),
						HumanReadableModTime: "2022-04-07 18:23:55 UTC",
						FileType:             rview.FileTypeText,
						CanPreview:           true,
						OriginalFileURL:      "/api/file/main.go?mod_time=1649355835",
						IconName:             "document",
					},
					{
						Filename:             "test.gif",
						Size:                 1833,
						HumanReadableSize:    "1.79 KiB",
						ModTime:              mustParseTime(t, "2023-01-01 15:00:00"),
						HumanReadableModTime: "2023-01-01 15:00:00 UTC",
						FileType:             rview.FileTypeImage,
						CanPreview:           true,
						OriginalFileURL:      "/api/file/test.gif?mod_time=1672585200",
						ThumbnailURL:         "/api/thumbnail/test.gif?mod_time=1672585200",
						IconName:             "image",
					},
				},
			},
			dirInfo,
		)

		dirInfo = getDirInfo(t, "/Other/a%20&%20b/x/", "")
		r.Equal(
			web.DirInfo{
				Dir: "/Other/a & b/x/",
				Breadcrumbs: []web.DirBreadcrumb{
					{Link: "/ui/", Text: "Home"},
					{Link: "/ui/Other/", Text: "Other"},
					{Link: "/ui/Other/a%20&%20b/", Text: "a & b"},
					{Link: "/ui/Other/a%20&%20b/x/", Text: "x"},
				},
				Entries: []web.DirEntry{
					{
						Filename:             "x & y.txt",
						Size:                 4,
						HumanReadableSize:    "4 B",
						ModTime:              mustParseTime(t, "2023-06-06 00:00:13"),
						HumanReadableModTime: "2023-06-06 00:00:13 UTC",
						FileType:             rview.FileTypeText,
						CanPreview:           true,
						OriginalFileURL:      "/api/file/Other/a%20&%20b/x/x%20&%20y.txt?mod_time=1686009613",
						IconName:             "document",
					},
				},
			},
			dirInfo,
		)
		dirInfo = getDirInfo(t, "/Other/spe%27sial%20%21%20cha%3Cracters/x/y/", "")
		r.Equal(
			web.DirInfo{
				Dir: "/Other/spe'sial ! cha<racters/x/y/",
				Breadcrumbs: []web.DirBreadcrumb{
					{Link: "/ui/", Text: "Home"},
					{Link: "/ui/Other/", Text: "Other"},
					{Link: "/ui/Other/spe%27sial%20%21%20cha%3Cracters/", Text: "spe'sial ! cha<racters"},
					{Link: "/ui/Other/spe%27sial%20%21%20cha%3Cracters/x/", Text: "x"},
					{Link: "/ui/Other/spe%27sial%20%21%20cha%3Cracters/x/y/", Text: "y"},
				},
				Entries: []web.DirEntry{
					{
						Filename:             "a + b.txt",
						Size:                 0,
						HumanReadableSize:    "0 B",
						ModTime:              mustParseTime(t, "2022-09-08 11:37:02"),
						HumanReadableModTime: "2022-09-08 11:37:02 UTC",
						FileType:             rview.FileTypeText,
						CanPreview:           true,
						OriginalFileURL:      "/api/file/Other/spe%27sial%20%21%20cha%3Cracters/x/y/a%20+%20b.txt?mod_time=1662637022",
						IconName:             "document",
					},
					{
						Filename:             "f>ile.txt",
						Size:                 0,
						HumanReadableSize:    "0 B",
						ModTime:              mustParseTime(t, "2022-09-08 11:37:02"),
						HumanReadableModTime: "2022-09-08 11:37:02 UTC",
						FileType:             rview.FileTypeText,
						CanPreview:           true,
						OriginalFileURL:      "/api/file/Other/spe%27sial%20%21%20cha%3Cracters/x/y/f%3Eile.txt?mod_time=1662637022",
						IconName:             "document",
					},
				},
			},
			dirInfo,
		)
	})

	t.Run("check sort", func(t *testing.T) {
		r := require.New(t)
		extractNames := func(info web.DirInfo) (res []string) {
			for _, e := range info.Entries {
				res = append(res, e.Filename)
			}
			return res
		}

		info := getDirInfo(t, "/Images/", "")
		r.Equal("", info.Sort)
		r.Equal("", info.Order)
		r.Equal(
			[]string{
				"Arts",
				"Photos",
				"asdfgh.heic",
				"birds-g64b44607c_640.jpg",
				"corgi-g4ea377693_640.jpg",
				"credits.txt",
				"horizontal.jpg",
				"qwerty.webp",
				"sky.avif",
				"vertical.jpg",
				"ytrewq.png",
				"zebra-g4e368da8d_640.jpg",
			},
			extractNames(info),
		)
		var canPreviewCount int
		for _, e := range info.Entries {
			if e.CanPreview {
				canPreviewCount++
			}
		}
		r.Equal(10, canPreviewCount)

		info = getDirInfo(t, "/Images/", "order=desc")
		r.Equal("", info.Sort)
		r.Equal("desc", info.Order)
		r.Equal(
			[]string{
				"zebra-g4e368da8d_640.jpg",
				"ytrewq.png",
				"vertical.jpg",
				"sky.avif",
				"qwerty.webp",
				"horizontal.jpg",
				"credits.txt",
				"corgi-g4ea377693_640.jpg",
				"birds-g64b44607c_640.jpg",
				"asdfgh.heic",
				"Photos",
				"Arts",
			},
			extractNames(info),
		)

		info = getDirInfo(t, "/Images/", "sort=size&order=desc")
		r.Equal("size", info.Sort)
		r.Equal("desc", info.Order)
		r.Equal(
			[]string{
				"zebra-g4e368da8d_640.jpg",
				"corgi-g4ea377693_640.jpg",
				"birds-g64b44607c_640.jpg",
				"horizontal.jpg",
				"vertical.jpg",
				"asdfgh.heic",
				"ytrewq.png",
				"sky.avif",
				"qwerty.webp",
				"credits.txt",
				"Photos",
				"Arts",
			},
			extractNames(info),
		)
	})

	t.Run("non-existent dirs", func(t *testing.T) {
		r := require.New(t)

		status, _, _ := makeRequest(t, "/ui/qwerty/")
		r.Equal(200, status)

		status, _, _ = makeRequest(t, "/api/dir/qwerty/")
		r.Equal(404, status)
	})
}

func TestAPI_GetFile(t *testing.T) {
	startTestRview()

	r := require.New(t)

	// No file.
	status, _, _ := makeRequest(t, "/api/file/Video/credits.txt1?mod_time=1662637030")
	r.Equal(404, status)

	// No mod_tim.
	status, _, _ = makeRequest(t, "/api/file/Video/credits.txt")
	r.Equal(400, status)

	// Wrong mod_time.
	status, _, _ = makeRequest(t, "/api/file/Video/credits.txt?mod_time=1662637030")
	r.Equal(500, status)

	// Check Content-Type and body of a text file.
	status, body, headers := makeRequest(t, "/api/file/Video/credits.txt?mod_time=1662637032")
	r.Equal(200, status)
	r.Contains(string(body), ".mp4")
	r.Equal("text/plain; charset=utf-8", headers.Get("Content-Type"))

	// Check Content-Type of a video.
	status, _, headers = makeRequest(t, "/api/file/Video/traffic-53902.mp4?mod_time=1662637022")
	r.Equal(200, status)
	r.Equal("video/mp4", headers.Get("Content-Type"))

	// Check Content-Type of an audio.
	status, _, headers = makeRequest(t, "/api/file/Audio/click-button-140881.mp3?mod_time=1660004130")
	r.Equal(200, status)
	r.Equal("audio/mpeg", headers.Get("Content-Type"))
}

func TestAPI_Thumbnails(t *testing.T) {
	startTestRview()

	r := require.New(t)

	generateImage := func(name string, size int, modTimeMonth time.Month) (modTime time.Time) {
		filepath := path.Join("testdata", name)

		// Generate an image with a pattern to make the final file bigger.
		img := image.NewRGBA(image.Rect(0, 0, size, size))
		for i := 0; i < size; i += 3 {
			for j := 0; j < size; j += 3 {
				img.Set(i, j, image.White)
			}
		}

		f, err := os.Create(filepath)
		r.NoError(err)
		t.Cleanup(func() {
			err := os.Remove(f.Name())
			if !errors.Is(err, os.ErrNotExist) {
				r.NoError(err)
			}
		})

		err = jpeg.Encode(f, img, &jpeg.Options{Quality: 95})
		r.NoError(err)

		r.NoError(f.Close())

		modTime = time.Date(2023, modTimeMonth, 11, 0, 0, 0, 0, time.UTC)
		err = os.Chtimes(filepath, modTime, modTime)
		r.NoError(err)

		return modTime
	}

	const (
		testFile      = "Other/test-thumbnails/cloudy-g1a943401b_640.png"
		generatedFile = "Other/test-thumbnails/generated.jpeg"
	)

	// Generate large image.
	generatedFileModTime := generateImage(generatedFile, 500, time.March)

	testFileTime := mustParseTime(t, TestDataModTimes[testFile])
	testFileThumbnailURL := "/api/thumbnail/Other/test-thumbnails/cloudy-g1a943401b_640.png?mod_time=" + strconv.Itoa(int(testFileTime.Unix()))

	generatedFileThumbnailURL := "/api/thumbnail/Other/test-thumbnails/generated.thumbnail.jpeg?mod_time=" + strconv.Itoa(int(generatedFileModTime.Unix()))

	// Thumbnails were not generated yet.
	for _, url := range []string{
		testFileThumbnailURL,
		generatedFileThumbnailURL,
	} {
		status, _, _ := makeRequest(t, url)
		r.Equal(404, status)
	}

	// Requesting dir info must send tasks to generate thumbnails.
	info := getDirInfo(t, path.Dir(testFile), "")
	r.NotEmpty(info.Entries)
	for _, entry := range info.Entries {
		switch entry.Filename {
		case path.Base(testFile):
			r.Equal(testFileThumbnailURL, entry.ThumbnailURL)
		case path.Base(generatedFile):
			r.Equal(generatedFileThumbnailURL, entry.ThumbnailURL)
		case "credits.txt":
			// Ok
		default:
			t.Fatalf("unexpected file %q", entry.Filename)
		}
	}

	// Thumbnails must be ready.
	var generatedFileThumbnailSize int
	for _, largeFile := range []bool{false, true} {
		fileURL, thumbnailURL, modTime := testFile, testFileThumbnailURL, testFileTime
		if largeFile {
			fileURL, thumbnailURL, modTime = generatedFile, generatedFileThumbnailURL, generatedFileModTime
		}
		fileURL = "/api/file/" + fileURL + "?mod_time=" + strconv.Itoa(int(modTime.Unix()))

		status, thumbnailBody, _ := makeRequest(t, thumbnailURL)
		r.Equal(200, status)

		status, fileBody, _ := makeRequest(t, fileURL)
		r.Equal(200, status)

		if !largeFile {
			r.Equal(len(thumbnailBody), len(fileBody))
		} else {
			r.Less(len(thumbnailBody), len(fileBody))
		}

		if thumbnailURL == generatedFileThumbnailURL {
			generatedFileThumbnailSize = len(thumbnailBody)
		}
	}

	// We should return different thumbnail url for edited file.

	generateImage(generatedFile, 499, time.April)

	var newGeneratedFileThumbnailURL string
	info = getDirInfo(t, path.Dir(testFile), "")
	for _, entry := range info.Entries {
		if entry.Filename == path.Base(generatedFile) {
			newGeneratedFileThumbnailURL = entry.ThumbnailURL
		}
	}
	r.NotEmpty(newGeneratedFileThumbnailURL)
	r.NotEqual(newGeneratedFileThumbnailURL, generatedFileThumbnailURL)

	status, newThumbnailBody, _ := makeRequest(t, newGeneratedFileThumbnailURL)
	r.Equal(200, status)
	r.NotEqual(len(newThumbnailBody), generatedFileThumbnailSize)
}

func TestAPI_Search(t *testing.T) {
	startTestRview()

	search := func(t *testing.T, s string) (dirs, files []string) {
		r := require.New(t)

		status, body, _ := makeRequest(t, "/api/search?search="+url.QueryEscape(s))
		r.Equal(200, status)

		var resp web.SearchResponse
		err := json.Unmarshal(body, &resp)
		r.NoError(err)

		for _, d := range resp.Dirs {
			r.True(strings.HasSuffix(d.WebURL, "/"))
			r.NotEmpty(d.Icon)

			dirs = append(dirs, d.Path)
		}
		for _, f := range resp.Files {
			r.Contains(f.WebURL, "?preview=")
			r.NotEmpty(f.Icon)

			files = append(files, f.Path)
		}
		return dirs, files
	}

	r := require.New(t)

	dirs, files := search(t, "birds")
	r.Empty(dirs)
	r.Equal([]string{"Images/birds-g64b44607c_640.jpg"}, files)

	dirs, files = search(t, "credits.txt")
	r.Empty(dirs)
	r.Equal([]string{
		"Audio/credits.txt",
		"Images/credits.txt",
		"Other/test-thumbnails/credits.txt",
		"Video/credits.txt",
	}, files)

	dirs, files = search(t, "audio credits.txt")
	r.Empty(dirs)
	r.Equal([]string{"Audio/credits.txt"}, files)

	dirs, files = search(t, "tests")
	r.Equal([]string{"Other/test-thumbnails/"}, dirs)
	r.Len(files, 3)
}

func getDirInfo(t *testing.T, dir string, query string) (res web.DirInfo) {
	t.Helper()

	status, body, _ := makeRequest(t, path.Join("/api/dir", dir)+"/?"+query)
	require.Equal(t, 200, status)

	err := json.Unmarshal(body, &res)
	require.NoError(t, err)

	return res
}

func makeRequest(t *testing.T, path string) (status int, body []byte, header http.Header) {
	t.Helper()

	req, err := http.NewRequestWithContext(context.Background(), "GET", rviewAPIAddr+path, nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err = io.ReadAll(resp.Body)
	require.NoError(t, err)

	return resp.StatusCode, body, resp.Header
}

func mustParseTime(t *testing.T, s string) time.Time {
	res, err := time.Parse(time.DateTime, s)
	require.NoError(t, err)
	return res.UTC()
}
