package web

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/ShoshinNikita/rview/cache"
	"github.com/ShoshinNikita/rview/config"
	"github.com/ShoshinNikita/rview/pkg/util/testutil"
	"github.com/ShoshinNikita/rview/rview"
	"github.com/ShoshinNikita/rview/static"
	"github.com/ShoshinNikita/rview/thumbnails"
)

func TestMain(m *testing.M) {
	err := static.Prepare()
	if err != nil {
		panic(err)
	}

	os.Exit(m.Run())
}

func TestServer_handleDir(t *testing.T) {
	parseTime := func(t *testing.T, s string) time.Time {
		res, err := time.Parse(time.RFC3339, s)
		testutil.NoError(t, err)
		return res.UTC()
	}

	tests := []struct {
		reqPath        string
		wantStatusCode int
		wantErrorBody  string
		wantInfo       Info
	}{
		{
			reqPath:        "/api/dir/images/arts/",
			wantStatusCode: http.StatusOK,
			wantInfo: Info{
				Dir: "/images/arts",
				Breadcrumbs: []Breadcrumb{
					{Link: "/ui/", Text: "Root"},
					{Link: "/ui/images/", Text: "images"},
					{Link: "/ui/images/arts/", Text: "arts"},
				},
				Entries: []Entry{
					{
						Filename:             "todo",
						IsDir:                true,
						ModTime:              parseTime(t, "2023-02-28T01:07:12+04:00"),
						HumanReadableModTime: "2023-02-27 21:07:12 UTC",
						DirURL:               "/api/dir/images/arts/todo/",
						WebDirURL:            "/ui/images/arts/todo/",
						IconName:             "folder.svg",
					},
					{
						Filename:             "1.txt",
						IsDir:                false,
						Size:                 0,
						HumanReadableSize:    "0 B",
						FileType:             rview.FileTypeText,
						CanPreview:           true,
						ModTime:              parseTime(t, "2023-02-28T01:06:22+04:00"),
						HumanReadableModTime: "2023-02-27 21:06:22 UTC",
						OriginalFileURL:      "/api/file/images/arts/1.txt?mod_time=1677531982",
						IconName:             "document.svg",
					},
					{
						Filename:             "2023-01-22_09-05-47-875.jpg",
						IsDir:                false,
						Size:                 2838056,
						HumanReadableSize:    "2.71 MiB",
						ModTime:              parseTime(t, "2023-01-22T13:05:50+04:00"),
						HumanReadableModTime: "2023-01-22 09:05:50 UTC",
						FileType:             rview.FileTypeImage,
						CanPreview:           true,
						OriginalFileURL:      "/api/file/images/arts/2023-01-22_09-05-47-875.jpg?mod_time=1674378350",
						ThumbnailURL:         "/api/thumbnail/images/arts/2023-01-22_09-05-47-875.jpg?mod_time=1674378350",
						IconName:             "image.svg",
					},
					{
						Filename:             "Screenshot_20220424-191722.jpg",
						IsDir:                false,
						Size:                 1359962,
						HumanReadableSize:    "1.3 MiB",
						ModTime:              parseTime(t, "2022-04-24T19:17:22+04:00"),
						HumanReadableModTime: "2022-04-24 15:17:22 UTC",
						FileType:             rview.FileTypeImage,
						CanPreview:           true,
						OriginalFileURL:      "/api/file/images/arts/Screenshot_20220424-191722.jpg?mod_time=1650813442",
						ThumbnailURL:         "/api/thumbnail/images/arts/Screenshot_20220424-191722.jpg?mod_time=1650813442",
						IconName:             "image.svg",
					},
				},
			},
		},
		{
			reqPath:        "/api/dir/images/arts/todo/",
			wantStatusCode: http.StatusOK,
			wantInfo: Info{
				Dir: "/images/arts/todo",
				Breadcrumbs: []Breadcrumb{
					{Link: "/ui/", Text: "Root"},
					{Link: "/ui/images/", Text: "images"},
					{Link: "/ui/images/arts/", Text: "arts"},
					{Link: "/ui/images/arts/todo/", Text: "todo"},
				},
				Entries: []Entry{
					{
						Filename:             "test ' special ! characters.ico",
						IsDir:                false,
						Size:                 0,
						HumanReadableSize:    "0 B",
						ModTime:              parseTime(t, "2023-02-28T01:06:45+04:00"),
						HumanReadableModTime: "2023-02-27 21:06:45 UTC",
						FileType:             rview.FileTypeImage,
						CanPreview:           false,
						OriginalFileURL:      "/api/file/images/arts/todo/test%20%27%20special%20%21%20characters.ico?mod_time=1677532005",
						IconName:             "image.svg",
					},
				},
			},
		},
		{
			reqPath:        "/api/dir/test's/test's/x/?sort=namedirfirst&order=desc",
			wantStatusCode: http.StatusOK,
			wantInfo: Info{
				Sort:  "namedirfirst",
				Order: "desc",
				Dir:   "/test's/test's/x",
				Breadcrumbs: []Breadcrumb{
					{Link: "/ui/", Text: "Root"},
					{Link: "/ui/test%27s/", Text: "test's"},
					{Link: "/ui/test%27s/test%27s/", Text: "test's"},
					{Link: "/ui/test%27s/test%27s/x/", Text: "x"},
				},
				Entries: []Entry{},
			},
		},
		{
			reqPath:        "/api/dir/404",
			wantStatusCode: 500,
			wantErrorBody:  `couldn't get rclone info: got unexpected status code from rclone: 404, body: "Directory not found"` + "\n",
		},
		{
			reqPath:        "/api/dir/500",
			wantStatusCode: 500,
			wantErrorBody:  `couldn't get rclone info: got unexpected status code from rclone: 500, body: "Internal Server Error"` + "\n",
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		data, err := os.ReadFile("testdata" + r.URL.Path + "resp.json")
		testutil.NoError(t, err)
		w.Write(data)
	})
	mux.HandleFunc("/404", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("Directory not found"))
	})
	mux.HandleFunc("/500", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	})
	testServer := httptest.NewServer(mux)

	for _, tt := range tests {
		tt := tt
		t.Run("", func(t *testing.T) {
			stub := newThumbnailServiceStub()
			s := NewServer(config.Config{}, stub)
			s.rcloneURL = mustParseURL(testServer.URL)

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", tt.reqPath, nil)

			s.handleDir(w, req)

			testutil.Equal(t, tt.wantStatusCode, w.Result().StatusCode)
			if tt.wantStatusCode != http.StatusOK {
				testutil.Equal(t, tt.wantErrorBody, w.Body.String())
				return
			}

			var gotInfo Info
			err := json.NewDecoder(w.Body).Decode(&gotInfo)
			testutil.NoError(t, err)
			testutil.Equal(t, tt.wantInfo, gotInfo)
		})
	}
}

func TestServer_sendGenerateThumbnailTasks(t *testing.T) {
	t.Parallel()

	stub := newThumbnailServiceStub()
	s := NewServer(config.Config{}, stub)

	zeroModTime := time.Unix(0, 0)

	gotInfo := s.sendGenerateThumbnailTasks(Info{
		Entries: []Entry{
			{filepath: "a.txt", ModTime: zeroModTime},
			{filepath: "b.jpg", ModTime: zeroModTime},
			{filepath: "c.png", ModTime: zeroModTime},
			{filepath: "c.bmp", ModTime: zeroModTime},
			{filepath: "d.zip", ModTime: zeroModTime},
			{filepath: "error.jpg", ModTime: zeroModTime},
			{filepath: "resized.jpg", ModTime: zeroModTime},
		},
		dirURL: mustParseURL("/"),
	})
	testutil.Equal(t, 3, stub.taskCount)

	testutil.Equal(t,
		[]Entry{
			{filepath: "a.txt", ModTime: zeroModTime}, // no thumbnail: text file
			{filepath: "b.jpg", ModTime: zeroModTime, ThumbnailURL: "/api/thumbnail/b.jpg?mod_time=0"},
			{filepath: "c.png", ModTime: zeroModTime, ThumbnailURL: "/api/thumbnail/c.png?mod_time=0"},
			{filepath: "c.bmp", ModTime: zeroModTime},     // no thumbnail: unsupported image
			{filepath: "d.zip", ModTime: zeroModTime},     // no thumbnail: archive
			{filepath: "error.jpg", ModTime: zeroModTime}, // no thumbnail: got error
			{filepath: "resized.jpg", ModTime: zeroModTime, ThumbnailURL: "/api/thumbnail/resized.jpg?mod_time=0"},
		},
		gotInfo.Entries,
	)
}

type thumbnailServiceStub struct {
	s rview.ThumbnailService

	taskCount int
}

func newThumbnailServiceStub() *thumbnailServiceStub {
	return &thumbnailServiceStub{
		s: thumbnails.NewThumbnailService(cache.NewNoopCache(), 0),
	}
}

func (s *thumbnailServiceStub) IsThumbnailReady(id rview.FileID) bool {
	return id.GetName() == "resized.jpg"
}

func (s *thumbnailServiceStub) SendTask(id rview.FileID, openFileFn rview.OpenFileFn) error {
	s.taskCount++

	if id.GetName() == "error.jpg" {
		return errors.New("error")
	}
	return nil
}

func (s *thumbnailServiceStub) CanGenerateThumbnail(id rview.FileID) bool {
	return s.s.CanGenerateThumbnail(id)
}

func (s *thumbnailServiceStub) OpenThumbnail(ctx context.Context, id rview.FileID) (io.ReadCloser, error) {
	return s.s.OpenThumbnail(ctx, id)
}

func (s *thumbnailServiceStub) Shutdown(ctx context.Context) error {
	return s.s.Shutdown(ctx)
}
