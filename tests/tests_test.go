package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
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
	"Other/spe'sial ! characters/x/y/file.txt": "2022-09-08 11:37:02",
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
		// Enable thumbnails but use 0 workers to not process any tasks.
		// TODO: increase workers count?
		Thumbnails:             true,
		ThumbnailsWorkersCount: 0,
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
	parseTime := func(t *testing.T, s string) time.Time {
		res, err := time.Parse(time.DateTime, s)
		require.NoError(t, err)
		return res.UTC()
	}

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
						ModTime:              parseTime(t, "2022-08-09 00:15:30"),
						HumanReadableModTime: "2022-08-09 00:15:30 UTC",
						DirURL:               "/api/dir/Audio/",
						WebDirURL:            "/ui/Audio/",
						IconName:             "folder-audio.svg",
					},
					{
						Filename:             "Images",
						IsDir:                true,
						ModTime:              parseTime(t, "2023-01-01 18:35:00"),
						HumanReadableModTime: "2023-01-01 18:35:00 UTC",
						DirURL:               "/api/dir/Images/",
						WebDirURL:            "/ui/Images/",
						IconName:             "folder-images.svg",
					},
					{
						Filename:             "Other",
						IsDir:                true,
						ModTime:              parseTime(t, "2022-09-08 11:37:02"),
						HumanReadableModTime: "2022-09-08 11:37:02 UTC",
						DirURL:               "/api/dir/Other/",
						WebDirURL:            "/ui/Other/",
						IconName:             "folder-other.svg",
					},
					{
						Filename:             "Video",
						IsDir:                true,
						ModTime:              parseTime(t, "2022-09-08 11:37:02"),
						HumanReadableModTime: "2022-09-08 11:37:02 UTC",
						DirURL:               "/api/dir/Video/",
						WebDirURL:            "/ui/Video/",
						IconName:             "folder-video.svg",
					},
					{
						Filename:             "archive.7z",
						Size:                 0,
						HumanReadableSize:    "0 B",
						ModTime:              parseTime(t, "2022-04-07 05:23:55"),
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
						ModTime:              parseTime(t, "2023-02-27 15:00:00"),
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
						ModTime:              parseTime(t, "2022-04-07 18:23:55"),
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
						ModTime:              parseTime(t, "2023-01-01 15:00:00"),
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
						ModTime:              parseTime(t, "2022-09-08 11:37:02"),
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

		// TODO: use 404
		status, _, _ := makeRequest(t, "/ui/qwerty/")
		r.Equal(500, status)

		// TODO: use 404
		status, _, _ = makeRequest(t, "/api/dir/qwerty/")
		r.Equal(500, status)
	})
}

func TestGetFile(t *testing.T) {
	r := require.New(t)

	// No file.
	// TODO: use 404
	status, _, _ := makeRequest(t, "/api/file/Video/credits.txt1?mod_time=1662637030")
	r.Equal(500, status)

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
