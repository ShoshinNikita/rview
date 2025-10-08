package tests

import (
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
	"path/filepath"
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

			resp, err := http.DefaultClient.Get(rcloneAddr)
			if resp != nil {
				resp.Body.Close()
			}
			if err == nil && resp.StatusCode == 200 {
				break
			}
		}

		// Wait for components to be ready.
		for i := range 10 {
			if i != 0 {
				time.Sleep(100 * time.Millisecond)
			}

			resp, err := http.DefaultClient.Get(rviewAPIAddr + "/api/search?search=test")
			if resp != nil {
				resp.Body.Close()
			}
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
				DirCount:      4,
				FileCount:     4,
				TotalFileSize: 2849,
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
						OriginalFileURL:      "/api/file/archive.7z?mod_time=1649309035&size=0",
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
						OriginalFileURL:      "/api/file/Lorem%20ipsum.txt?mod_time=1677510000&size=943",
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
						OriginalFileURL:      "/api/file/main.go?mod_time=1649355835&size=73",
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
						OriginalFileURL:      "/api/file/test.gif?mod_time=1672585200&size=1833",
						ThumbnailURL:         "/api/thumbnail/test.gif?mod_time=1672585200&size=1833",
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
				FileCount:     1,
				TotalFileSize: 4,
				Entries: []web.DirEntry{
					{
						Filename:             "x & y.txt",
						Size:                 4,
						HumanReadableSize:    "4 B",
						ModTime:              mustParseTime(t, "2023-06-06 00:00:13"),
						HumanReadableModTime: "2023-06-06 00:00:13 UTC",
						FileType:             rview.FileTypeText,
						CanPreview:           true,
						OriginalFileURL:      "/api/file/Other/a%20&%20b/x/x%20&%20y.txt?mod_time=1686009613&size=4",
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
				FileCount:     3,
				TotalFileSize: 0,
				Entries: []web.DirEntry{
					{
						Filename:             "100%.txt",
						Size:                 0,
						HumanReadableSize:    "0 B",
						ModTime:              mustParseTime(t, "2025-10-08 02:49:00"),
						HumanReadableModTime: "2025-10-08 02:49:00 UTC",
						FileType:             rview.FileTypeText,
						CanPreview:           true,
						OriginalFileURL:      "/api/file/Other/spe%27sial%20%21%20cha%3Cracters/x/y/100%25.txt?mod_time=1759891740&size=0",
						IconName:             "document",
					},
					{
						Filename:             "a + b.txt",
						Size:                 0,
						HumanReadableSize:    "0 B",
						ModTime:              mustParseTime(t, "2022-09-08 11:37:02"),
						HumanReadableModTime: "2022-09-08 11:37:02 UTC",
						FileType:             rview.FileTypeText,
						CanPreview:           true,
						OriginalFileURL:      "/api/file/Other/spe%27sial%20%21%20cha%3Cracters/x/y/a%20+%20b.txt?mod_time=1662637022&size=0",
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
						OriginalFileURL:      "/api/file/Other/spe%27sial%20%21%20cha%3Cracters/x/y/f%3Eile.txt?mod_time=1662637022&size=0",
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
				"vertical.jpg",
				"horizontal.jpg",
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

	// No such file.
	status, _, _ := makeRequest(t, "/api/file/Video/credits.txt1?mod_time=1662637030&size=0")
	r.Equal(404, status)

	// No mod_time.
	status, body, _ := makeRequest(t, "/api/file/Video/credits.txt")
	r.Equal(400, status)
	r.Contains(string(body), "invalid mod_time")

	// No size.
	status, body, _ = makeRequest(t, "/api/file/Video/credits.txt?mod_time=1662637030")
	r.Equal(400, status)
	r.Contains(string(body), "invalid size")

	// Wrong mod_time.
	status, body, _ = makeRequest(t, "/api/file/Video/credits.txt?mod_time=1662637030&size=162")
	r.Equal(500, status)
	r.Contains(string(body), "different mod time")

	// Wrong size.
	status, body, _ = makeRequest(t, "/api/file/Video/credits.txt?mod_time=1662637032&size=123")
	r.Equal(500, status)
	r.Contains(string(body), "different size")

	// Check Content-Type and body of a text file.
	status, body, headers := makeRequest(t, "/api/file/Video/credits.txt?mod_time=1662637032&size=162")
	r.Equal(200, status)
	r.Contains(string(body), ".mp4")
	r.Equal("text/plain; charset=utf-8", headers.Get("Content-Type"))

	// Check Content-Type of a video.
	status, _, headers = makeRequest(t, "/api/file/Video/traffic-53902.mp4?mod_time=1662637022&size=299776")
	r.Equal(200, status)
	r.Equal("video/mp4", headers.Get("Content-Type"))

	// Check Content-Type of an audio.
	status, _, headers = makeRequest(t, "/api/file/Audio/click-button-140881.mp3?mod_time=1660004130&size=15882")
	r.Equal(200, status)
	r.Equal("audio/mpeg", headers.Get("Content-Type"))

	t.Run("range request", func(t *testing.T) {
		status, _, headers := makeRequest(t, "/api/file/Video/credits.txt?mod_time=1662637032&size=162")
		r.Equal(200, status)
		r.Equal("bytes", headers.Get("Accept-Ranges"))

		header := http.Header{
			"Range": {"bytes=0-10"},
		}
		status, body, _ := makeRequest(t, "/api/file/Video/credits.txt?mod_time=1662637032&size=162", requestOptions{header: header})
		r.Equal(206, status)
		r.Equal("traffic-539", string(body))

		header = http.Header{
			"Range": {"bytes=3-16"},
		}
		status, body, _ = makeRequest(t, "/api/file/Video/credits.txt?mod_time=1662637032&size=162", requestOptions{header: header})
		r.Equal(206, status)
		r.Equal("ffic-53902.mp4", string(body))
	})
}

func TestAPI_Thumbnails(t *testing.T) {
	startTestRview()

	r := require.New(t)

	dir := filepath.Join("./testdata", "generated")
	err := os.MkdirAll(dir, 0700)
	r.NoError(err)
	t.Cleanup(func() {
		err := os.Remove(dir)
		r.NoError(err)
	})

	generateImage := func(name string, size int) {
		filepath := path.Join(dir, name)

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

		err = jpeg.Encode(f, img, &jpeg.Options{Quality: 100})
		r.NoError(err)
		r.NoError(f.Close())
	}

	generateImage("small.jpeg", 50)
	generateImage("medium.jpeg", 500)
	generateImage("large.jpeg", 1500)

	info := getDirInfo(t, "generated", "")
	r.Len(info.Entries, 3)
	for _, entry := range info.Entries {
		r.NotEmpty(entry.ThumbnailURL)

		status, smallRawThumbnail, header := makeRequest(t, entry.ThumbnailURL)
		r.Equal(200, status)
		r.Equal("image/jpeg", header.Get("Content-Type"))

		status, largeRawThumbnail, header := makeRequest(t, entry.ThumbnailURL+"&thumbnail_size=large")
		r.Equal(200, status)
		r.Equal("image/jpeg", header.Get("Content-Type"))

		switch entry.Filename {
		case "small.jpeg":
			r.Equal(len(smallRawThumbnail), int(entry.Size)) // the original image was small enough
		case "medium.jpeg":
			r.Less(len(smallRawThumbnail), int(entry.Size))         // image should be resized
			r.Equal(len(smallRawThumbnail), len(largeRawThumbnail)) // image resolution is small, no difference
		case "large.jpeg":
			r.Less(len(smallRawThumbnail), int(entry.Size))            // image should be resized
			r.NotEqual(len(smallRawThumbnail), len(largeRawThumbnail)) // enough image resolution to see the difference
		default:
			t.Fatalf("unexpected file %q", entry.Filename)
		}
	}
}

func TestAPI_Search(t *testing.T) {
	startTestRview()

	search := func(t *testing.T, s string) (dirs, files []string) {
		r := require.New(t)

		status, body, _ := makeRequest(t, "/api/search?limit=10&search="+url.QueryEscape(s))
		r.Equal(200, status)

		var resp web.SearchResponse
		err := json.Unmarshal(body, &resp)
		r.NoError(err)

		for _, h := range resp.Hits {
			if h.IsDir {
				r.True(strings.HasSuffix(h.WebURL, "/"))
				r.NotEmpty(h.Icon)
				dirs = append(dirs, h.Path)

			} else {
				r.Contains(h.WebURL, "?preview=")
				r.NotEmpty(h.Icon)

				files = append(files, h.Path)
			}
		}
		return dirs, files
	}

	r := require.New(t)

	dirs, files := search(t, "birds")
	r.Empty(dirs)
	r.Equal([]string{"/Images/birds-g64b44607c_640.jpg"}, files)

	dirs, files = search(t, "credits.txt")
	r.Empty(dirs)
	r.Equal([]string{
		"/Audio/credits.txt",
		"/Images/credits.txt",
		"/Other/test-thumbnails/credits.txt",
		"/Video/credits.txt",
	}, files)

	dirs, files = search(t, "audio credits.txt")
	r.Empty(dirs)
	r.Equal([]string{"/Audio/credits.txt"}, files)

	dirs, files = search(t, "tests")
	r.Equal([]string{"/Other/test-thumbnails/"}, dirs)
	r.Equal([]string{"/test.gif"}, files)
}

func getDirInfo(t *testing.T, dir string, query string) (res web.DirInfo) {
	t.Helper()

	status, body, _ := makeRequest(t, path.Join("/api/dir", dir)+"/?"+query)
	require.Equal(t, 200, status)

	err := json.Unmarshal(body, &res)
	require.NoError(t, err)

	return res
}

type requestOptions struct {
	header http.Header
}

func makeRequest(t *testing.T, path string, opts ...requestOptions) (status int, body []byte, header http.Header) {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), "GET", rviewAPIAddr+path, nil)
	require.NoError(t, err)
	if len(opts) > 0 {
		if len(opts) > 1 {
			t.Fatalf("opts can contain only 1 element, got %d", len(opts))
		}
		req.Header = opts[0].header
	}

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
