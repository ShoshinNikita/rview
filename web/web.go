package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
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
	"github.com/ShoshinNikita/rview/pkg/metrics"
	"github.com/ShoshinNikita/rview/pkg/rlog"
	"github.com/ShoshinNikita/rview/rview"
	"github.com/ShoshinNikita/rview/static"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Server struct {
	buildInfo config.BuildInfo
	rcloneURL *url.URL

	httpServer *http.Server
	httpClient *http.Client

	resizer rview.ImageResizer

	iconsFS     fs.FS
	fileIconsFS fs.FS
	templatesFS fs.FS
}

func NewServer(cfg config.Config, resizer rview.ImageResizer) (s *Server) {
	s = &Server{
		buildInfo: cfg.BuildInfo,
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
			handler = cacheMiddleware(30*24*time.Hour, cfg.ShortGitHash, handler)
		}
		handler = http.StripPrefix(pattern, handler)
		mux.Handle(pattern, handler)
	}

	// API
	mux.HandleFunc("/api/dir/", s.handleDir)
	mux.HandleFunc("/api/file/", s.handleFile)
	mux.HandleFunc("/api/thumbnail/", s.handleThumbnail)

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
	dir := strings.TrimPrefix(r.URL.Path, "/api/dir/")

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
	// significantly simplifies the development process.
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

	copyResponse(w, buf)
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

	var rcloneInfo RcloneInfo
	err = json.NewDecoder(resp.Body).Decode(&rcloneInfo)
	if err != nil {
		return RcloneInfo{}, fmt.Errorf("couldn't decode rclone response: %w", err)
	}

	// Rclone escapes the directory path and text of breadcrumbs: &#39; instead of ' and so on.
	// So, we have to unescape them to build valid links.
	rcloneInfo.Path = html.UnescapeString(rcloneInfo.Path)
	for i := range rcloneInfo.Breadcrumbs {
		rcloneInfo.Breadcrumbs[i].Text = html.UnescapeString(rcloneInfo.Breadcrumbs[i].Text)
	}

	return rcloneInfo, nil
}

func (s *Server) convertRcloneInfo(rcloneInfo RcloneInfo) (Info, error) {
	info := Info{
		BuildInfo: s.buildInfo,
		//
		Sort:  rcloneInfo.Sort,
		Order: rcloneInfo.Order,
		//
		dirURL: mustParseURL("/"),
	}

	for _, breadcrumb := range rcloneInfo.Breadcrumbs {
		if breadcrumb.Text == "" {
			continue
		}

		// Dir name must not be escaped.
		info.Dir = pkgPath.Join(info.Dir, breadcrumb.Text)

		// It doesn't make any sense to add another trailing slash (especially, escaped).
		if breadcrumb.Text != "/" {
			info.dirURL = info.dirURL.JoinPath(url.PathEscape(breadcrumb.Text))
		}
		// All directory urls must end with slash.
		info.dirURL = info.dirURL.JoinPath("/")

		text := breadcrumb.Text
		if text == "/" {
			text = "Root"
		}

		uiURL := mustParseURL("/ui").JoinPath(info.dirURL.String()).String()

		info.Breadcrumbs = append(info.Breadcrumbs, Breadcrumb{
			Link: uiURL,
			Text: text,
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
		filepath := pkgPath.Join(info.Dir, filename)

		var originalFileURL, dirURL, webDirURL string
		if entry.IsDir {
			escapedFilename := url.PathEscape(filename)

			dirURL = mustParseURL("/api/dir").JoinPath(info.dirURL.String(), escapedFilename, "/").String()
			webDirURL = mustParseURL("/ui").JoinPath(info.dirURL.String(), escapedFilename, "/").String()

		} else {
			id := rview.NewFileID(filepath, entry.ModTime)
			originalFileURL = fileIDToURL("/api/file", info.dirURL, id)
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
		if entry.IsDir {
			continue
		}

		id := rview.NewFileID(entry.filepath, entry.ModTime.Unix())
		if !s.resizer.CanResize(id) {
			continue
		}

		// TODO: limit max image size?

		thumbnailURL := fileIDToURL("/api/thumbnail", info.dirURL, id)

		if s.resizer.IsResized(id) {
			info.Entries[i].ThumbnailURL = thumbnailURL
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

		info.Entries[i].ThumbnailURL = thumbnailURL
	}

	return info
}

// handleFile proxy the original file from Rclone, copying some headers.
func (s *Server) handleFile(w http.ResponseWriter, r *http.Request) {
	fileID, err := fileIDFromRequest(r, "/api/file")
	if err != nil {
		writeBadRequestError(w, err.Error())
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

	copyResponse(w, rc)
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

// handleThumbnail returns the resized image.
func (s *Server) handleThumbnail(w http.ResponseWriter, r *http.Request) {
	fileID, err := fileIDFromRequest(r, "/api/thumbnail")
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

	contentType := mime.TypeByExtension(pkgPath.Ext(fileID.GetName()))
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	// Use mod time as a value for ETag.
	etag := strconv.Itoa(int(fileID.GetModTime().Unix()))
	setCacheHeaders(w, 30*24*time.Hour, etag)

	copyResponse(w, rc)
}

func fileIDToURL(prefix string, dirURL *url.URL, id rview.FileID) string {
	fileURL := mustParseURL(prefix).JoinPath(dirURL.String(), url.PathEscape(id.GetName()))

	query := fileURL.Query()
	query.Set("mod_time", strconv.FormatInt(id.GetModTime().Unix(), 10))
	fileURL.RawQuery = query.Encode()

	return fileURL.String()
}

func fileIDFromRequest(r *http.Request, endpointPrefix string) (rview.FileID, error) {
	path := strings.TrimPrefix(r.URL.Path, endpointPrefix)
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

func copyResponse(w http.ResponseWriter, src io.Reader) {
	_, err := io.Copy(w, src)
	if err != nil {
		writeInternalServerError(w, "couldn't write response: %s", err)
	}
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

func mustParseURL(raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil {
		panic(fmt.Errorf("couldn't parse %q: %w", raw, err))
	}
	return u
}
