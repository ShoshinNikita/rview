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
	"strconv"
	"strings"
	"time"

	"github.com/ShoshinNikita/rview/config"
	"github.com/ShoshinNikita/rview/pkg/rlog"
	"github.com/ShoshinNikita/rview/rview"
	"github.com/ShoshinNikita/rview/static"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Server struct {
	buildInfo config.BuildInfo

	httpServer *http.Server

	rclone           rview.Rclone
	thumbnailService rview.ThumbnailService
	searchService    rview.SearchService

	iconsFS     fs.FS
	fileIconsFS fs.FS
	templatesFS fs.FS
}

func NewServer(cfg config.Config, rclone rview.Rclone, thumbnailService rview.ThumbnailService, searchService rview.SearchService) (s *Server) {
	if cfg.ReadStaticFilesFromDisk {
		rlog.Info("static files will be read from disk")
	}

	s = &Server{
		buildInfo: cfg.BuildInfo,
		//
		rclone:           rclone,
		thumbnailService: thumbnailService,
		searchService:    searchService,
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
	mux.HandleFunc("/api/search", s.handleSearch)
	mux.HandleFunc("/api/search/refresh-indexes", s.handleRefreshIndexes)

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

	w.Header().Set("Content-Type", "text/html")
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
// the appropriate format. It also sends tasks to generate thumbnail for the images.
func (s *Server) getDirInfo(ctx context.Context, dir string, query url.Values) (DirInfo, error) {
	rcloneInfo, err := s.rclone.GetDirInfo(ctx, dir, query.Get("sort"), query.Get("order"))
	if err != nil {
		return DirInfo{}, fmt.Errorf("couldn't get rclone info: %w", err)
	}

	info, err := s.convertRcloneInfo(rcloneInfo)
	if err != nil {
		return DirInfo{}, fmt.Errorf("couldn't convert rclone info: %w", err)
	}

	info = s.sendGenerateThumbnailTasks(info)

	return info, nil
}

func (s *Server) convertRcloneInfo(rcloneInfo *rview.RcloneDirInfo) (DirInfo, error) {
	info := DirInfo{
		BuildInfo: s.buildInfo,
		//
		Sort:  rcloneInfo.Sort,
		Order: rcloneInfo.Order,
		// Always encode entries as a slice.
		Entries: []DirEntry{},
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

		info.Breadcrumbs = append(info.Breadcrumbs, DirBreadcrumb{
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
			return DirInfo{}, fmt.Errorf("invalid url %q: %w", entry.URL, err)
		}
		filepath := pkgPath.Join(info.Dir, filename)

		var (
			dirURL, webDirURL string
			//
			originalFileURL, humanReadableSize string
			fileType                           rview.FileType
			canPreview                         bool
		)
		if entry.IsDir {
			escapedFilename := url.PathEscape(filename)

			dirURL = mustParseURL("/api/dir").JoinPath(info.dirURL.String(), escapedFilename, "/").String()
			webDirURL = mustParseURL("/ui").JoinPath(info.dirURL.String(), escapedFilename, "/").String()

		} else {
			id := rview.NewFileID(filepath, entry.ModTime)

			originalFileURL = fileIDToURL("/api/file", info.dirURL, id)
			humanReadableSize = FormatFileSize(entry.Size)
			fileType = rview.GetFileType(id)

			switch fileType {
			case rview.FileTypeText:
				canPreview = true

			case rview.FileTypeImage:
				canPreview = s.thumbnailService.CanGenerateThumbnail(id)

			case rview.FileTypeAudio:
				switch id.GetExt() {
				case ".mp3", ".ogg", ".wav":
					canPreview = true
				}

			case rview.FileTypeVideo:
				switch id.GetExt() {
				case ".mp4", ".webm":
					canPreview = true
				}
			}
		}

		modTime := time.Unix(entry.ModTime, 0).UTC()
		info.Entries = append(info.Entries, DirEntry{
			filepath: filepath,
			//
			Filename:             filename,
			IsDir:                entry.IsDir,
			Size:                 entry.Size,
			HumanReadableSize:    humanReadableSize,
			ModTime:              modTime,
			HumanReadableModTime: FormatModTime(modTime),
			FileType:             fileType,
			CanPreview:           canPreview,
			//
			DirURL:          dirURL,
			WebDirURL:       webDirURL,
			OriginalFileURL: originalFileURL,
			IconName:        static.GetFileIcon(filename, entry.IsDir),
		})
	}
	return info, nil
}

func (s *Server) sendGenerateThumbnailTasks(info DirInfo) DirInfo {
	for i, entry := range info.Entries {
		if entry.IsDir {
			continue
		}

		id := rview.NewFileID(entry.filepath, entry.ModTime.Unix())
		if !s.thumbnailService.CanGenerateThumbnail(id) {
			continue
		}

		// TODO: limit max image size?

		thumbnailURL := fileIDToURL("/api/thumbnail", info.dirURL, id)

		if s.thumbnailService.IsThumbnailReady(id) {
			info.Entries[i].ThumbnailURL = thumbnailURL
			continue
		}

		openFile := func(ctx context.Context, id rview.FileID) (io.ReadCloser, error) {
			rc, _, err := s.rclone.GetFile(ctx, id)
			return rc, err
		}
		err := s.thumbnailService.SendTask(id, openFile)
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

	rc, rcloneHeaders, err := s.rclone.GetFile(r.Context(), fileID)
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
		contentType := mime.TypeByExtension(fileID.GetExt())
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
	}

	copyResponse(w, rc)
}

// handleThumbnail returns the thumbnail.
func (s *Server) handleThumbnail(w http.ResponseWriter, r *http.Request) {
	fileID, err := fileIDFromRequest(r, "/api/thumbnail")
	if err != nil {
		writeBadRequestError(w, err.Error())
		return
	}

	rc, err := s.thumbnailService.OpenThumbnail(r.Context(), fileID)
	if err != nil {
		writeBadRequestError(w, "couldn't open thumbnail: %s", err)
		return
	}
	defer rc.Close()

	contentType := mime.TypeByExtension(fileID.GetExt())
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	// Use mod time as a value for ETag.
	etag := strconv.Itoa(int(fileID.GetModTime().Unix()))
	setCacheHeaders(w, 30*24*time.Hour, etag)

	copyResponse(w, rc)
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	atoi := func(param string, defaultValue int) int {
		res, err := strconv.Atoi(r.FormValue(param))
		if err != nil {
			return defaultValue
		}
		return res
	}

	search := r.FormValue("search")
	minLength := s.searchService.GetMinSearchLength()
	if len([]rune(search)) < minLength {
		writeBadRequestError(w, `minimum "search" length is %d characters`, minLength)
		return
	}
	dirLimit := atoi("dir-limit", 3)
	fileLimit := atoi("file-limit", 5)

	dirs, files, err := s.searchService.Search(r.Context(), search, dirLimit, fileLimit)
	if err != nil {
		writeInternalServerError(w, "search failed: %s", err)
		return
	}

	convertSearchHits := func(hits []rview.SearchHit, isDir bool) []SearchHit {
		res := make([]SearchHit, 0, len(hits))
		for _, hit := range hits {
			var webURL string

			if isDir {
				webURL = mustParseURL("/ui").JoinPath(hit.Path, "/").String()

			} else {
				dir := pkgPath.Dir(hit.Path)
				filename := pkgPath.Base(hit.Path)

				u := mustParseURL("/ui").JoinPath(dir, "/")
				u.RawQuery = url.Values{
					"preview": []string{filename},
				}.Encode()
				webURL = u.String()
			}

			res = append(res, SearchHit{
				SearchHit: hit,
				WebURL:    webURL,
				Icon:      static.GetFileIcon(hit.Path, isDir),
			})
		}
		return res
	}
	resp := SearchResponse{
		Dirs:  convertSearchHits(dirs, true),
		Files: convertSearchHits(files, false),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleRefreshIndexes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		code := http.StatusMethodNotAllowed
		http.Error(w, http.StatusText(code), code)
		return
	}

	// Use background context because index refresh can take a while, and
	// we don't want to interrupt this process.
	ctx := context.Background()
	err := s.searchService.RefreshIndexes(ctx)
	if err != nil {
		writeInternalServerError(w, "couldn't refresh indexes: %s", err)
		return
	}
	w.WriteHeader(http.StatusOK)
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
