package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	pkgPath "path"
	pkgFilepath "path/filepath"
	"strconv"
	"time"

	"github.com/ShoshinNikita/rview/resizer"
)

type Server struct {
	httpServer    *http.Server
	httpClient    *http.Client
	rcloneBaseURL *url.URL
	resizer       ImageResizer
}

type ImageResizer interface {
	CanResize(filepath string) bool
	IsResized(filepath string, modTime time.Time) bool
	OpenResized(ctx context.Context, filepath string, modTime time.Time) (io.ReadCloser, error)
	Resize(filepath string, modTime time.Time, getImageFn resizer.GetFileFn) error
}

func NewServer(port int, rcloneBaseURL *url.URL, resizer ImageResizer) (s *Server) {
	s = &Server{
		rcloneBaseURL: rcloneBaseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		resizer: resizer,
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/dir", s.handleDir)
	mux.HandleFunc("/file", s.handleFile)
	mux.HandleFunc("/thumbnail", s.handleThumbnail)

	s.httpServer = &http.Server{
		Addr:    ":" + strconv.Itoa(port),
		Handler: mux,
	}

	return s
}

func (s *Server) Start() error {
	log.Printf("start web server on %q", s.httpServer.Addr)

	err := s.httpServer.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

// handleDir requests the directory information from Rclone and converts it into
// the appropriate format. It also sends resize tasks for the images.
func (s *Server) handleDir(w http.ResponseWriter, r *http.Request) {
	dir := r.FormValue("dir")

	rcloneInfo, err := s.getRcloneInfo(r.Context(), dir, r.URL.RawQuery)
	if err != nil {
		writeInternalServerError(w, "couldn't get rclone info: %s", err)
		return
	}
	info, err := s.convertRcloneInfo(rcloneInfo)
	if err != nil {
		writeInternalServerError(w, "couldn't convert rclone info: %s", err)
		return
	}

	info = s.sendResizeImageTasks(info)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

func (s *Server) getRcloneInfo(ctx context.Context, path, query string) (RcloneInfo, error) {
	rcloneURL := s.rcloneBaseURL.JoinPath(path)
	rcloneURL.RawQuery = query
	req, err := http.NewRequestWithContext(ctx, "GET", rcloneURL.String(), nil)
	if err != nil {
		return RcloneInfo{}, fmt.Errorf("couldn't prepare request: %w", err)
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return RcloneInfo{}, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return RcloneInfo{}, fmt.Errorf("got unexpected status code from rclone: %d, body: %q", resp.StatusCode, body)
	}

	var rcloneInfo RcloneInfo
	err = json.NewDecoder(resp.Body).Decode(&rcloneInfo)
	if err != nil {
		return RcloneInfo{}, fmt.Errorf("couldn't decode rclone response: %w", err)
	}

	return rcloneInfo, nil
}

func (*Server) convertRcloneInfo(rcloneInfo RcloneInfo) (Info, error) {
	info := Info{
		Sort:  rcloneInfo.Sort,
		Order: rcloneInfo.Order,
	}
	for _, breadcrumb := range rcloneInfo.Breadcrumbs {
		if breadcrumb.Text == "" {
			continue
		}
		info.Breadcrumbs = append(info.Breadcrumbs, Breadcrumb{
			Link: pkgPath.Join(rcloneInfo.Name, breadcrumb.Link),
			Text: breadcrumb.Text,
		})
	}
	for _, entry := range rcloneInfo.Entries {
		if entry.URL == "" {
			continue
		}

		filename, err := url.QueryUnescape(pkgPath.Clean(entry.URL))
		if err != nil {
			return Info{}, fmt.Errorf("invalid url %q: %w", entry.URL, err)
		}
		filepath := pkgPath.Join(rcloneInfo.Name, filename)

		var originalFileURL, dirURL string
		if entry.IsDir {
			dirURL = (&url.URL{
				Path: "/dir",
				RawQuery: (url.Values{
					"dir": []string{filepath + "/"},
				}).Encode(),
			}).String()

		} else {
			originalFileURL = (&url.URL{
				Path: "/file",
				RawQuery: (url.Values{
					"filepath": []string{filepath},
				}).Encode(),
			}).String()
		}

		info.Entries = append(info.Entries, Entry{
			filepath: filepath,
			//
			Filename: filename,
			IsDir:    entry.IsDir,
			Size:     entry.Size,
			ModTime:  time.Unix(entry.ModTime, 0),
			//
			DirURL:          dirURL,
			OriginalFileURL: originalFileURL,
		})
	}
	return info, nil
}

func (s *Server) sendResizeImageTasks(info Info) Info {
	for i, entry := range info.Entries {
		if entry.IsDir {
			continue
		}
		if !s.resizer.CanResize(entry.filepath) {
			continue
		}
		// TODO: limit max image size?

		thumbnailURL := &url.URL{
			Path: "/thumbnail",
			RawQuery: (url.Values{
				"mod_time": []string{strconv.Itoa(int(entry.ModTime.Unix()))},
				"filepath": []string{entry.filepath},
			}).Encode(),
		}

		if s.resizer.IsResized(entry.filepath, entry.ModTime) {
			info.Entries[i].ThumbnailURL = thumbnailURL.String()
			continue
		}

		getFile := func(ctx context.Context, path string) (io.ReadCloser, error) {
			rc, _, err := s.getFile(ctx, path)
			return rc, err
		}
		err := s.resizer.Resize(entry.filepath, entry.ModTime, getFile)
		if err != nil {
			log.Printf("couldn't start resizing for file %q: %s", entry.filepath, err)
			continue
		}

		info.Entries[i].ThumbnailURL = thumbnailURL.String()
	}

	return info
}

// handleFile proxy the original file from Rclone, copying some headers.
func (s *Server) handleFile(w http.ResponseWriter, r *http.Request) {
	filepath := r.FormValue("filepath")
	if filepath == "" {
		writeBadRequestError(w, "filepath is required")
		return
	}

	rc, rcloneHeaders, err := s.getFile(r.Context(), filepath)
	if err != nil {
		writeInternalServerError(w, "couldn't get file: %s", err)
		return
	}
	defer rc.Close()

	for _, headerName := range []string{
		"Content-Type",
		"Content-Length",
		"Last-Modified",
		"Date",
	} {
		for _, value := range rcloneHeaders.Values(headerName) {
			w.Header().Add(headerName, value)
		}
	}
	if w.Header().Get("Content-Type") == "" {
		contentType := mime.TypeByExtension(pkgFilepath.Ext(filepath))
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
	}

	io.Copy(w, rc)
}

func (s *Server) getFile(ctx context.Context, path string) (io.ReadCloser, http.Header, error) {
	rcloneURL := s.rcloneBaseURL.JoinPath(path)
	req, err := http.NewRequestWithContext(ctx, "GET", rcloneURL.String(), nil)
	if err != nil {
		return nil, nil, fmt.Errorf("couldn't prepare request: %w", err)
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("request failed: %w", err)
	}

	return resp.Body, resp.Header, nil
}

// handleThumbnail returns the resized image.
func (s *Server) handleThumbnail(w http.ResponseWriter, r *http.Request) {
	path := r.FormValue("filepath")
	rawModTime := r.FormValue("mod_time")
	if path == "" || rawModTime == "" {
		writeBadRequestError(w, "both filepath and mod_time are required")
		return
	}
	modTime, err := strconv.Atoi(rawModTime)
	if err != nil {
		writeBadRequestError(w, "mod_time must be a number")
		return
	}

	rc, err := s.resizer.OpenResized(r.Context(), path, time.Unix(int64(modTime), 0))
	if err != nil {
		writeBadRequestError(w, "couldn't open resized image: %s", err)
		return
	}
	defer rc.Close()

	contentType := mime.TypeByExtension(pkgFilepath.Ext(path))
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	io.Copy(w, rc)
}

func writeBadRequestError(w http.ResponseWriter, format string, a ...any) {
	writeError(w, http.StatusBadRequest, format, a...)
}

func writeInternalServerError(w http.ResponseWriter, format string, a ...any) {
	writeError(w, http.StatusInternalServerError, format, a...)
}

func writeError(w http.ResponseWriter, code int, format string, a ...any) {
	http.Error(w, fmt.Sprintf(format, a...), code)
}
