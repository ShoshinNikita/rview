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

	"github.com/ShoshinNikita/rview/rview"
)

const maxFileSizeForCache = 512 << 10 // 512 KiB

type Server struct {
	httpServer    *http.Server
	httpClient    *http.Client
	rcloneBaseURL *url.URL
	resizer       rview.ImageResizer
	cache         rview.Cache
}

func NewServer(port int, rcloneBaseURL *url.URL, resizer rview.ImageResizer, cache rview.Cache) (s *Server) {
	s = &Server{
		rcloneBaseURL: rcloneBaseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		resizer: resizer,
		cache:   cache,
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
			id := rview.NewFileID(filepath, entry.ModTime)
			originalFileURL = (&url.URL{
				Path:     "/file",
				RawQuery: fileIDToQuery(make(url.Values), id).Encode(),
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
		id := rview.NewFileID(entry.filepath, entry.ModTime.Unix())

		if entry.IsDir {
			continue
		}
		if !s.resizer.CanResize(id) {
			continue
		}
		// TODO: limit max image size?

		thumbnailURL := &url.URL{
			Path:     "/thumbnail",
			RawQuery: fileIDToQuery(make(url.Values), id).Encode(),
		}

		if s.resizer.IsResized(id) {
			info.Entries[i].ThumbnailURL = thumbnailURL.String()
			continue
		}

		openFile := func(ctx context.Context, id rview.FileID) (io.ReadCloser, error) {
			rc, _, err := s.getFile(ctx, id)
			return rc, err
		}
		err := s.resizer.Resize(id, openFile)
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
	fileID, err := fileIDFromQuery(r)
	if err != nil {
		writeBadRequestError(w, err.Error())
		return
	}

	rc, err := s.cache.Open(fileID)
	if err == nil {
		log.Printf("serve file %q from cache", fileID)
		defer rc.Close()

		contentType := mime.TypeByExtension(pkgFilepath.Ext(fileID.GetName()))
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		w.Header().Set("Last-Modified", fileID.GetModTime().Format(http.TimeFormat))

		io.Copy(w, rc)
		return
	}

	rc, rcloneHeaders, err := s.getFile(r.Context(), fileID)
	if err != nil {
		writeInternalServerError(w, "couldn't get file: %s", err)
		return
	}
	defer rc.Close()

	fileModTime, err := time.Parse(http.TimeFormat, rcloneHeaders.Get("Last-Modified"))
	if err != nil {
		writeInternalServerError(w, "rclone response must have valid Last-Modified header: %s", err)
		return
	}
	if !fileModTime.Equal(fileID.GetModTime()) {
		writeInternalServerError(w,
			"rclone file and requested file have different mod times: %q, %q",
			fileModTime, fileID.GetModTime(),
		)
		return
	}

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
		contentType := mime.TypeByExtension(pkgFilepath.Ext(fileID.GetName()))
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
	}

	writer, close := s.getFileWriter(w, rcloneHeaders, fileID)
	defer close()

	io.Copy(writer, rc)
}

func (s *Server) getFile(ctx context.Context, id rview.FileID) (io.ReadCloser, http.Header, error) {
	rcloneURL := s.rcloneBaseURL.JoinPath(id.GetPath())
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

func (s *Server) getFileWriter(w http.ResponseWriter, rcloneHeaders http.Header, fileID rview.FileID) (_ io.Writer, close func() error) {
	close = func() error { return nil }

	rawSize := rcloneHeaders.Get("Content-Length")
	if rawSize == "" {
		log.Printf(`file %q doesn't have "Content-Length" header, skip caching`, fileID)
		return w, close
	}
	size, err := strconv.Atoi(rawSize)
	if err != nil {
		log.Printf(`couldn't parse value of "Content-Length" of file %q: %s`, fileID, err)
		return w, close
	}

	if size > maxFileSizeForCache {
		return w, close
	}

	cacheWriter, err := s.cache.GetSaveWriter(fileID)
	if err != nil {
		// We can serve the file. So, just log the error.
		log.Printf("couldn't get cache writer for file %q: %s", fileID, err)
		return w, close
	}

	log.Printf("save file %q to cache", fileID)

	res := io.MultiWriter(w, cacheWriter)

	return res, cacheWriter.Close
}

// handleThumbnail returns the resized image.
func (s *Server) handleThumbnail(w http.ResponseWriter, r *http.Request) {
	fileID, err := fileIDFromQuery(r)
	if err != nil {
		writeBadRequestError(w, err.Error())
		return
	}

	rc, err := s.resizer.OpenResized(r.Context(), fileID)
	if err != nil {
		writeBadRequestError(w, "couldn't open resized image: %s", err)
		return
	}
	defer rc.Close()

	contentType := mime.TypeByExtension(pkgFilepath.Ext(fileID.GetName()))
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	io.Copy(w, rc)
}

func fileIDToQuery(query url.Values, id rview.FileID) url.Values {
	query.Set("filepath", id.GetPath())
	query.Set("mod_time", strconv.FormatInt(id.GetModTime().Unix(), 10))
	return query
}

func fileIDFromQuery(r *http.Request) (rview.FileID, error) {
	path := r.FormValue("filepath")
	if path == "" {
		return rview.FileID{}, errors.New("filepath can't be empty")
	}
	rawModTime := r.FormValue("mod_time")
	modTime, err := strconv.ParseInt(rawModTime, 10, 64)
	if err != nil {
		return rview.FileID{}, fmt.Errorf("invalid mod_time: %w", err)
	}

	return rview.NewFileID(path, modTime), nil
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
