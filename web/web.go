package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	pkgPath "path"
	pkgFilepath "path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ShoshinNikita/rview/config"
	"github.com/ShoshinNikita/rview/metrics"
	"github.com/ShoshinNikita/rview/rlog"
	"github.com/ShoshinNikita/rview/rview"
	"github.com/ShoshinNikita/rview/static"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const maxFileSizeForCache = 512 << 10 // 512 KiB

type Server struct {
	cfg       config.Config
	rcloneURL *url.URL

	httpServer *http.Server
	httpClient *http.Client

	resizer rview.ImageResizer
	cache   rview.Cache

	iconsFS     fs.FS
	fileIconsFS fs.FS
	templatesFS fs.FS
}

func NewServer(cfg config.Config, resizer rview.ImageResizer, cache rview.Cache) (s *Server) {
	s = &Server{
		cfg: cfg,
		rcloneURL: &url.URL{
			Scheme: "http",
			Host:   "localhost:" + strconv.Itoa(cfg.RclonePort),
		},
		//
		httpClient: &http.Client{
			Timeout: time.Minute,
		},
		//
		resizer: resizer,
		cache:   cache,
		//
		iconsFS:     static.NewIconsFS(cfg.ReadStaticFilesFromDisk),
		fileIconsFS: static.NewFileIconsFS(cfg.ReadStaticFilesFromDisk),
		templatesFS: static.NewTemplatesFS(cfg.ReadStaticFilesFromDisk),
	}

	mux := http.NewServeMux()

	// UI
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, "/ui/", http.StatusSeeOther)
	})
	mux.HandleFunc("/ui/", s.handleUI)

	// Static
	for pattern, fs := range map[string]fs.FS{
		"/static/icons/":     s.iconsFS,
		"/static/fileicons/": s.fileIconsFS,
		"/static/styles/":    static.NewStylesFS(cfg.ReadStaticFilesFromDisk),
		"/static/js/":        static.NewScriptsFS(cfg.ReadStaticFilesFromDisk),
	} {
		handler := http.FileServer(http.FS(fs))
		if !cfg.ReadStaticFilesFromDisk {
			handler = cacheMiddleware(14*24*time.Hour, cfg.ShortGitHash, handler)
		}
		handler = http.StripPrefix(pattern, handler)
		mux.Handle(pattern, handler)
	}

	// API
	mux.HandleFunc("/api/dir", s.handleDir)
	mux.HandleFunc("/api/file", s.handleFile)
	mux.HandleFunc("/api/thumbnail", s.handleThumbnail)

	// Debug
	mux.Handle("/debug/metrics", promhttp.Handler())

	handler := loggingMiddleware(mux)

	s.httpServer = &http.Server{
		Addr:              ":" + strconv.Itoa(cfg.ServerPort),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	return s
}

func (s *Server) Start() error {
	rlog.Infof("start web server on %q", s.httpServer.Addr)

	err := s.httpServer.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) handleDir(w http.ResponseWriter, r *http.Request) {
	dir := r.FormValue("dir")

	info, err := s.getDirInfo(r.Context(), dir, r.URL.Query())
	if err != nil {
		writeInternalServerError(w, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

func (s *Server) handleUI(w http.ResponseWriter, r *http.Request) {
	dir := strings.TrimPrefix(r.URL.Path, "/ui")
	if !strings.HasSuffix(dir, "/") {
		dir += "/"
	}

	info, err := s.getDirInfo(r.Context(), dir, r.URL.Query())
	if err != nil {
		writeInternalServerError(w, err.Error())
		return
	}

	s.executeTemplate(w, "index.html", info)
}

func (s *Server) executeTemplate(w http.ResponseWriter, name string, data any) {
	// Parse templates every time because it doesn't affect performance but
	// significantly increases the development process.
	template, err := template.New("base").
		Funcs(template.FuncMap{
			"attr": func(s string) template.HTMLAttr {
				return template.HTMLAttr(s)
			},
			"embedIcon": func(name string) (template.HTML, error) {
				return embedIcon(s.iconsFS, name)
			},
			"embedFileIcon": func(name string) (template.HTML, error) {
				return embedIcon(s.fileIconsFS, name)
			},
		}).
		ParseFS(s.templatesFS, "index.html", "preview.html", "footer.html")
	if err != nil {
		writeInternalServerError(w, "couldn't parse templates: %s", err)
		return
	}

	buf := bytes.NewBuffer(nil)
	err = template.ExecuteTemplate(buf, name, data)
	if err != nil {
		writeInternalServerError(w, "couldn't execute templates: %s", err)
		return
	}

	io.Copy(w, buf)
}

func embedIcon(fs fs.FS, name string) (template.HTML, error) {
	if !strings.HasSuffix(name, ".svg") {
		name += ".svg"
	}
	f, err := fs.Open(name)
	if err != nil {
		return "", fmt.Errorf("couldn't open icon %q: %w", name, err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return "", fmt.Errorf("couldn't read icon %q: %w", name, err)
	}
	return template.HTML(data), nil
}

// getDirInfo requests the directory information from Rclone and converts it into
// the appropriate format. It also sends resize tasks for the images.
func (s *Server) getDirInfo(ctx context.Context, dir string, query url.Values) (Info, error) {
	rcloneInfo, err := s.getRcloneInfo(ctx, dir, query)
	if err != nil {
		return Info{}, fmt.Errorf("couldn't get rclone info: %w", err)
	}

	info, err := s.convertRcloneInfo(rcloneInfo)
	if err != nil {
		return Info{}, fmt.Errorf("couldn't convert rclone info: %w", err)
	}

	info = s.sendResizeImageTasks(info)

	return info, nil
}

func (s *Server) getRcloneInfo(ctx context.Context, path string, query url.Values) (RcloneInfo, error) {
	now := time.Now()
	defer func() {
		dur := time.Since(now)

		metrics.RcloneResponseTime.Observe(dur.Seconds())
		rlog.Debugf("rclone info for %q was loaded in %s", path, dur)
	}()

	rcloneURL := s.rcloneURL.JoinPath(path)
	rcloneURL.RawQuery = url.Values{
		"sort":  query["sort"],
		"order": query["order"],
	}.Encode()
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

	buf := bytes.NewBuffer(nil)
	body := io.TeeReader(resp.Body, buf)

	var rcloneInfo RcloneInfo
	err = json.NewDecoder(body).Decode(&rcloneInfo)
	if err != nil {
		if strings.Contains(buf.String(), "<!DOCTYPE html>") {
			return RcloneInfo{}, errors.New(`rclone returned HTML response, did you use "--template" option?`)
		}
		return RcloneInfo{}, fmt.Errorf("couldn't decode rclone response: %w", err)
	}

	return rcloneInfo, nil
}

func (s *Server) convertRcloneInfo(rcloneInfo RcloneInfo) (Info, error) {
	info := Info{
		BuildInfo: s.cfg.BuildInfo,
		//
		Sort:  rcloneInfo.Sort,
		Order: rcloneInfo.Order,
	}

	var dirParts []string
	for _, breadcrumb := range rcloneInfo.Breadcrumbs {
		if breadcrumb.Text == "" {
			continue
		}

		link, err := url.JoinPath("/ui", rcloneInfo.Name, breadcrumb.Link)
		if err != nil {
			return Info{}, fmt.Errorf("couldn't prepare breadcrumb link: %w", err)
		}

		text := breadcrumb.Text
		if text == "/" {
			text = "Root"
		}

		info.Breadcrumbs = append(info.Breadcrumbs, Breadcrumb{
			Link: link,
			Text: text,
		})
		dirParts = append(dirParts, breadcrumb.Text)
	}
	info.Dir = pkgPath.Join(dirParts...)

	for _, entry := range rcloneInfo.Entries {
		if entry.URL == "" {
			continue
		}

		filename, err := url.QueryUnescape(pkgPath.Clean(entry.URL))
		if err != nil {
			return Info{}, fmt.Errorf("invalid url %q: %w", entry.URL, err)
		}
		filepath := pkgPath.Join(rcloneInfo.Name, filename)

		var originalFileURL, dirURL, webDirURL string
		if entry.IsDir {
			dirURL = (&url.URL{
				Path: "/api/dir",
				RawQuery: (url.Values{
					"dir": []string{filepath + "/"},
				}).Encode(),
			}).String()

			webDirURL, err = url.JoinPath("/ui", filepath+"/")
			if err != nil {
				return Info{}, fmt.Errorf("couldn't prepare web directory url: %w", err)
			}

		} else {
			id := rview.NewFileID(filepath, entry.ModTime)
			originalFileURL = (&url.URL{
				Path:     "/api/file",
				RawQuery: fileIDToQuery(make(url.Values), id).Encode(),
			}).String()
		}

		modTime := time.Unix(entry.ModTime, 0)
		info.Entries = append(info.Entries, Entry{
			filepath: filepath,
			//
			Filename:             filename,
			IsDir:                entry.IsDir,
			Size:                 entry.Size,
			HumanReadableSize:    FormatFileSize(entry.Size),
			ModTime:              modTime,
			HumanReadableModTime: FormatModTime(modTime),
			//
			DirURL:          dirURL,
			WebDirURL:       webDirURL,
			OriginalFileURL: originalFileURL,
			IconName:        static.GetFileIcon(filename, entry.IsDir),
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
			Path:     "/api/thumbnail",
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
			rlog.Errorf("couldn't start resizing for file %q: %s", entry.filepath, err)
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
		rlog.Debugf("serve file %q from cache", fileID)
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
	defer close() //nolint:errcheck

	_, err = io.Copy(writer, rc)
	if err != nil {
		rlog.Errorf("couldn't serve file %q: %s", fileID, err)
	}
}

func (s *Server) getFile(ctx context.Context, id rview.FileID) (io.ReadCloser, http.Header, error) {
	rcloneURL := s.rcloneURL.JoinPath(id.GetPath())
	req, err := http.NewRequestWithContext(ctx, "GET", rcloneURL.String(), nil)
	if err != nil {
		return nil, nil, fmt.Errorf("couldn't prepare request: %w", err)
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("request failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("got invalid status code: %d", resp.StatusCode)
	}

	return resp.Body, resp.Header, nil
}

func (s *Server) getFileWriter(w http.ResponseWriter, rcloneHeaders http.Header, fileID rview.FileID) (_ io.Writer, close func() error) {
	close = func() error { return nil }

	rawSize := rcloneHeaders.Get("Content-Length")
	if rawSize == "" {
		rlog.Debugf(`file %q doesn't have "Content-Length" header, skip caching`, fileID)
		return w, close
	}
	size, err := strconv.Atoi(rawSize)
	if err != nil {
		rlog.Debugf(`couldn't parse value of "Content-Length" of file %q: %s`, fileID, err)
		return w, close
	}

	if size > maxFileSizeForCache {
		rlog.Debugf("don't cache too large file %q (%.2f MiB)", fileID, float64(maxFileSizeForCache)/(1<<20))
		return w, close
	}

	cacheWriter, err := s.cache.GetSaveWriter(fileID)
	if err != nil {
		// We can serve the file. So, just log the error.
		rlog.Debugf("couldn't get cache writer for file %q: %s", fileID, err)
		return w, close
	}

	rlog.Debugf("save file %q to cache", fileID)

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
	// Use mod time as a value for ETag.
	etag := strconv.Itoa(int(fileID.GetModTime().Unix()))
	setCacheHeaders(w, 24*time.Hour, etag)

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
	msg := fmt.Sprintf(format, a...)

	rlog.Errorf("internal error: %s", msg)
	writeError(w, http.StatusInternalServerError, msg)
}

func writeError(w http.ResponseWriter, code int, format string, a ...any) {
	http.Error(w, fmt.Sprintf(format, a...), code)
}
