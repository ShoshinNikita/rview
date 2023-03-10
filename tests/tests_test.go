package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ShoshinNikita/rview/cmd"
	"github.com/ShoshinNikita/rview/config"
	"github.com/ShoshinNikita/rview/rview"
	"github.com/ShoshinNikita/rview/web"
	"github.com/stretchr/testify/require"
)

var TestDataModTimes = map[string]string{
	"archive.7z":      "2022-04-07 05:23:55",
	"Lorem ipsum.txt": "2023-02-27 15:00:00",
	"main.go":         "2022-04-07 18:23:55",
	"test.gif":        "2023-01-01 15:00:00",
	//
	"Audio/":                        "2022-08-09 00:15:30",
	"Audio/click-button-140881.mp3": "2022-08-09 00:15:30",
	"Audio/credits.txt":             "2022-08-09 00:15:38",
	//
	"Images/":                         "2023-01-01 18:35:00",
	"Images/birds-g64b44607c_640.jpg": "2019-05-15 06:30:09",
	"Images/corgi-g4ea377693_640.jpg": "2023-01-01 18:35:00",
	"Images/credits.txt":              "2023-01-01 18:36:00",
	"Images/horizontal.jpg":           "2023-01-01 15:00:00",
	"Images/vertical.jpg":             "2023-01-01 15:00:00",
	"Images/zebra-g4e368da8d_640.jpg": "2023-01-05 16:00:37",
	//
	"Video/":                  "2022-09-08 11:37:02",
	"Video/credits.txt":       "2022-09-08 11:37:12",
	"Video/traffic-53902.mp4": "2022-09-08 11:37:02",
	//
	"Other/": "2022-09-08 11:37:02",
	"Other/spe'sial ! characters/x/y/file.txt":        "2022-09-08 11:37:02",
	"Other/test-thumbnails/cloudy-g1a943401b_640.png": "2022-09-11 18:35:04",
	"Other/test-thumbnails/credits.txt":               "2022-09-11 18:35:04",
}

var APIAddr string

func TestMain(m *testing.M) {
	for path, rawModTime := range TestDataModTimes {
		modTime, err := time.Parse(time.DateTime, rawModTime)
		if err != nil {
			panic(fmt.Errorf("couldn't parse testdata mod time: %w", err))
		}

		modTime = modTime.UTC()
		err = os.Chtimes("testdata/"+path, modTime, modTime)
		if err != nil {
			panic(fmt.Errorf("couldn't change mod time of %q: %w", path, err))
		}
	}
	tempDir, err := os.MkdirTemp("", "rview-tests-*")
	if err != nil {
		panic(fmt.Errorf("couldn't create temp dir: %w", err))
	}

	cfg := config.Config{
		ServerPort: mustGetFreePort(),
		Dir:        tempDir,
		//
		RcloneTarget: "./testdata",
		RclonePort:   mustGetFreePort(),
		//
		Thumbnails:             true,
		ThumbnailsWorkersCount: 1,
		//
		DebugLogLevel: true,
	}
	APIAddr = fmt.Sprintf("http://localhost:%d", cfg.ServerPort)
	rcloneAddr := fmt.Sprintf("http://localhost:%d", cfg.RclonePort)

	rview := cmd.NewRview(cfg)
	if err := rview.Prepare(); err != nil {
		panic(fmt.Errorf("couldn't prepare rview: %w", err))
	}
	done := rview.Start(func() {
		panic(fmt.Errorf("rview error"))
	})

	// Wait for rclone to start.
	for i := 0; i < 10; i++ {
		if i != 0 {
			time.Sleep(20 * time.Millisecond)
		}

		resp, err := http.DefaultClient.Get(rcloneAddr) //nolint:noctx
		if err == nil && resp.StatusCode == 200 {
			break
		}
	}

	// Wait for components to be ready.
	time.Sleep(100 * time.Millisecond)

	code := m.Run()

	err = rview.Shutdown(context.Background())
	if err != nil {
		code = 1
		log.Printf("shutdown error: %s", err)
	}

	<-done

	os.Exit(code)
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

func TestGetDirInfo(t *testing.T) {
	t.Run("check full info", func(t *testing.T) {
		r := require.New(t)

		dirInfo := getDirInfo(t, "/", "")
		r.Equal(
			web.DirInfo{
				Dir: "/",
				Breadcrumbs: []web.DirBreadcrumb{
					{Link: "/ui/", Text: "Root"},
				},
				Entries: []web.DirEntry{
					{
						Filename:             "Audio",
						IsDir:                true,
						ModTime:              mustParseTime(t, "2022-08-09 00:15:30"),
						HumanReadableModTime: "2022-08-09 00:15:30 UTC",
						DirURL:               "/api/dir/Audio/",
						WebDirURL:            "/ui/Audio/",
						IconName:             "folder-audio.svg",
					},
					{
						Filename:             "Images",
						IsDir:                true,
						ModTime:              mustParseTime(t, "2023-01-01 18:35:00"),
						HumanReadableModTime: "2023-01-01 18:35:00 UTC",
						DirURL:               "/api/dir/Images/",
						WebDirURL:            "/ui/Images/",
						IconName:             "folder-images.svg",
					},
					{
						Filename:             "Other",
						IsDir:                true,
						ModTime:              mustParseTime(t, "2022-09-08 11:37:02"),
						HumanReadableModTime: "2022-09-08 11:37:02 UTC",
						DirURL:               "/api/dir/Other/",
						WebDirURL:            "/ui/Other/",
						IconName:             "folder-other.svg",
					},
					{
						Filename:             "Video",
						IsDir:                true,
						ModTime:              mustParseTime(t, "2022-09-08 11:37:02"),
						HumanReadableModTime: "2022-09-08 11:37:02 UTC",
						DirURL:               "/api/dir/Video/",
						WebDirURL:            "/ui/Video/",
						IconName:             "folder-video.svg",
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
						IconName:             "zip.svg",
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
						IconName:             "document.svg",
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
						IconName:             "go.svg",
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
						IconName:             "image.svg",
					},
				},
			},
			dirInfo,
		)

		dirInfo = getDirInfo(t, "/Other/spe%27sial%20%21%20characters/x/y/", "")
		r.Equal(
			web.DirInfo{
				Dir: "/Other/spe'sial ! characters/x/y",
				Breadcrumbs: []web.DirBreadcrumb{
					{Link: "/ui/", Text: "Root"},
					{Link: "/ui/Other/", Text: "Other"},
					{Link: "/ui/Other/spe%27sial%20%21%20characters/", Text: "spe'sial ! characters"},
					{Link: "/ui/Other/spe%27sial%20%21%20characters/x/", Text: "x"},
					{Link: "/ui/Other/spe%27sial%20%21%20characters/x/y/", Text: "y"},
				},
				Entries: []web.DirEntry{
					{
						Filename:             "file.txt",
						Size:                 0,
						HumanReadableSize:    "0 B",
						ModTime:              mustParseTime(t, "2022-09-08 11:37:02"),
						HumanReadableModTime: "2022-09-08 11:37:02 UTC",
						FileType:             rview.FileTypeText,
						CanPreview:           true,
						OriginalFileURL:      "/api/file/Other/spe%27sial%20%21%20characters/x/y/file.txt?mod_time=1662637022",
						IconName:             "document.svg",
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
				"birds-g64b44607c_640.jpg",
				"corgi-g4ea377693_640.jpg",
				"credits.txt",
				"horizontal.jpg",
				"vertical.jpg",
				"zebra-g4e368da8d_640.jpg",
			},
			extractNames(info),
		)

		info = getDirInfo(t, "/Images/", "order=desc")
		r.Equal("", info.Sort)
		r.Equal("desc", info.Order)
		r.Equal(
			[]string{
				"zebra-g4e368da8d_640.jpg",
				"vertical.jpg",
				"horizontal.jpg",
				"credits.txt",
				"corgi-g4ea377693_640.jpg",
				"birds-g64b44607c_640.jpg",
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
				"credits.txt",
			},
			extractNames(info),
		)
	})

	t.Run("non-existent dirs", func(t *testing.T) {
		r := require.New(t)

		status, _, _ := makeRequest(t, "/ui/qwerty/")
		r.Equal(404, status)

		status, _, _ = makeRequest(t, "/api/dir/qwerty/")
		r.Equal(404, status)
	})
}

func TestGetFile(t *testing.T) {
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

func TestThumbnails(t *testing.T) {
	r := require.New(t)

	const (
		testFile      = "Other/test-thumbnails/cloudy-g1a943401b_640.png"
		generatedFile = "Other/test-thumbnails/generated.jpeg"
	)

	// Generate large image.
	var generatedFileTime time.Time
	func() {
		filepath := path.Join("testdata", generatedFile)

		img := image.NewRGBA(image.Rect(0, 0, 4000, 4000))
		f, err := os.Create(filepath)
		r.NoError(err)

		err = jpeg.Encode(f, img, &jpeg.Options{Quality: 1})
		r.NoError(err)

		r.NoError(f.Close())
		t.Cleanup(func() {
			r.NoError(os.Remove(f.Name()))
		})

		stats, err := os.Stat(filepath)
		r.NoError(err)
		generatedFileTime = stats.ModTime()
	}()

	testFileTime := mustParseTime(t, TestDataModTimes[testFile])
	testFileThumbnailURL := "/api/thumbnail/" + testFile + "?mod_time=" + strconv.Itoa(int(testFileTime.Unix()))

	generatedFileThumbnailURL := "/api/thumbnail/" + generatedFile + "?mod_time=" + strconv.Itoa(int(generatedFileTime.Unix()))

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
	for thumbnailURL, originalFileUsed := range map[string]bool{
		testFileThumbnailURL:      true,
		generatedFileThumbnailURL: false,
	} {
		status, thumbnailBody, _ := makeRequest(t, thumbnailURL)
		r.Equal(200, status)

		fileURL := strings.Replace(thumbnailURL, "/api/thumbnail/", "/api/file/", 1)
		status, fileBody, _ := makeRequest(t, fileURL)
		r.Equal(200, status)

		if originalFileUsed {
			r.Equal(len(thumbnailBody), len(fileBody))
		} else {
			r.Less(len(thumbnailBody), len(fileBody))
		}
	}
}

func TestSearch(t *testing.T) {
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
			r.True(strings.Contains(f.WebURL, "?preview="))
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

	req, err := http.NewRequestWithContext(context.Background(), "GET", APIAddr+path, nil)
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
