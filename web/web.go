package web

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net/http"
	"net/http/pprof"
	"net/url"
	pkgPath "path"
	"strconv"
	"strings"
	"time"

	"github.com/ShoshinNikita/rview/pkg/cache"
	"github.com/ShoshinNikita/rview/pkg/misc"
	"github.com/ShoshinNikita/rview/pkg/rlog"
	"github.com/ShoshinNikita/rview/rclone"
	"github.com/ShoshinNikita/rview/rview"
	"github.com/ShoshinNikita/rview/search"
	"github.com/ShoshinNikita/rview/static"
	"github.com/ShoshinNikita/rview/thumbnails"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Server struct {
	cfg rview.Config

	httpServer *http.Server

	rclone           *rclone.Rclone
	thumbnailService ThumbnailService
	searchService    *search.Service

	iconsFS     fs.FS
	templatesFS fs.FS
}

type ThumbnailService interface {
	CanGenerateThumbnail(rview.FileID) bool
	OpenThumbnail(context.Context, rview.FileID, thumbnails.ThumbnailSize) (rc io.ReadCloser, contentType string, err error)
}

func NewServer(cfg rview.Config, rclone *rclone.Rclone, thumbnailService ThumbnailService, searchService *search.Service) (s *Server) {
	if cfg.ReadStaticFilesFromDisk {
		rlog.Info("static files will be read from disk")
	}

	s = &Server{
		cfg: cfg,
		//
		rclone:           rclone,
		thumbnailService: thumbnailService,
		searchService:    searchService,
		//
		iconsFS:     static.NewIconsFS(cfg.ReadStaticFilesFromDisk),
		templatesFS: static.NewTemplatesFS(cfg.ReadStaticFilesFromDisk),
	}

	mux := http.NewServeMux()

	// UI
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, "/ui/", http.StatusSeeOther)
	})
	mux.HandleFunc("GET /ui/", s.handleUI)

	// Static
	for pattern, fs := range map[string]fs.FS{
		"/static/icons/": s.iconsFS,
		"/static/css/":   static.NewCssFS(cfg.ReadStaticFilesFromDisk),
		"/static/js/":    static.NewScriptsFS(cfg.ReadStaticFilesFromDisk),
		"/static/pwa/":   static.NewPwaFS(cfg.ReadStaticFilesFromDisk),
	} {
		handler := http.FileServer(http.FS(fs))
		if !cfg.ReadStaticFilesFromDisk {
			handler = cacheMiddleware(30*24*time.Hour, cfg.BuildInfo.ShortGitHash, handler)
		}
		handler = http.StripPrefix(pattern, handler)
		mux.Handle("GET "+pattern, handler)
	}

	// API
	mux.HandleFunc("GET /api/dir/", s.handleDir)
	mux.HandleFunc("GET /api/file/", s.handleFile)
	mux.HandleFunc("GET /api/thumbnail/", s.handleThumbnail)
	mux.HandleFunc("GET /api/search", s.handleSearch)
	mux.HandleFunc("POST /api/search/refresh-index", s.handleRefreshIndex)

	// Prometheus Metrics
	mux.Handle("GET /debug/metrics", promhttp.Handler())

	// Pprof
	mux.HandleFunc("GET /debug/pprof/", pprof.Index)
	mux.HandleFunc("GET /debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("GET /debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("GET /debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("GET /debug/pprof/trace", pprof.Trace)

	handler := loggingMiddleware(mux)

	s.httpServer = &http.Server{
		Addr:              ":" + strconv.Itoa(cfg.ServerPort),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	return s
}

func (s *Server) Start() error {
	rlog.Infof(`start web server on "http://localhost:%d"`, s.cfg.ServerPort)

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
		writeInternalServerError(w, "couldn't get dir info: %s", err)
		return
	}
	if info.IsNotFound {
		writeError(w, http.StatusNotFound, "dir %q not found", dir)
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
		writeInternalServerError(w, "couldn't get dir info: %s", err)
		return
	}

	s.executeTemplate(w, "index.html", info)
}

func (s *Server) executeTemplate(w http.ResponseWriter, name string, data any) {
	// Parse templates every time because it doesn't affect performance but
	// significantly simplifies the development process.
	template, err := template.New("base").
		Funcs(template.FuncMap{
			"add": func(a, b int) int {
				return a + b
			},
			"trim": func(s string, maxSize int) string {
				runes := []rune(s)
				if len(runes) > maxSize {
					return string(runes[:maxSize]) + "…"
				}
				return s
			},
			"prepareStaticLink": func(rawURL string) (string, error) {
				hash := s.cfg.BuildInfo.ShortGitHash
				if s.cfg.ReadStaticFilesFromDisk {
					// Every time generate random hash.
					data := make([]byte, 4)
					_, err := rand.Read(data)
					if err != nil {
						return "", fmt.Errorf("couldn't read rand data: %w", err)
					}
					hash = "from-disk-" + hex.EncodeToString(data)
				}

				u, err := url.Parse(rawURL)
				if err != nil {
					return "", fmt.Errorf("invalid url %q: %w", rawURL, err)
				}
				query := u.Query()
				query.Add("hash", hash)
				u.RawQuery = query.Encode()

				return u.String(), nil
			},
			"attr": func(s string) template.HTMLAttr {
				return template.HTMLAttr(s)
			},
			"js": func(s string) template.JS {
				return template.JS(s)
			},
			"jsEscape": template.JSEscapeString,
			"marshalJSON": func(v any) (template.JSStr, error) {
				res, err := json.Marshal(v)
				if err != nil {
					return "", err
				}
				return template.JSStr(res), nil
			},
			"embedIcon": func(name string) (template.HTML, error) {
				return s.embedIcon(static.FeatherIconsPack, name)
			},
			"embedFileIcon": func(name string) (template.HTML, error) {
				return s.embedIcon(static.MaterialIconsPack, name)
			},
			"dict": func(kvs ...any) (map[string]any, error) {
				if len(kvs)%2 != 0 {
					return nil, errors.New("number of args must be even")
				}

				res := make(map[string]any)
				for i := 0; i < len(kvs); i += 2 {
					k, ok := kvs[i].(string)
					if !ok {
						return nil, fmt.Errorf("keys must be string (arg #%d)", i)
					}
					v := kvs[i+1]

					res[k] = v
				}
				return res, nil
			},
			"formatSize":    misc.FormatFileSize,
			"formatModTime": misc.FormatModTime,
		}).
		ParseFS(s.templatesFS, "index.html", "preview.html", "footer.html", "search-results.html", "entry.html")
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
	io.Copy(w, buf)
}

func (s *Server) embedIcon(pack static.IconPack, name string) (template.HTML, error) {
	if !strings.HasSuffix(name, ".svg") {
		name += ".svg"
	}
	f, err := s.iconsFS.Open(pkgPath.Join(string(pack), name))
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
	dir = pkgPath.Clean(dir)
	if dir == "." {
		dir = "/"
	}

	// Dir must start and end with a slash.
	if !strings.HasPrefix(dir, "/") {
		dir = "/" + dir
	}
	if !strings.HasSuffix(dir, "/") {
		dir += "/"
	}

	var isNotFound bool

	rcloneInfo, err := s.rclone.GetDirInfo(ctx, dir, query.Get("sort"), query.Get("order"))
	if rclone.IsNotFoundError(err) {
		// It's hard to replicate the logic of "DirInfo" preparation. Therefore, just
		// set error to nil and init RcloneDirInfo with the predefined values.
		err = nil
		isNotFound = true

		rcloneInfo = &rclone.DirInfo{
			Breadcrumbs: []rclone.DirBreadcrumb{
				{Text: "/"},   // for link to Home
				{Text: "???"}, // indicate that something went wrong
			},
		}
	}
	if err != nil {
		return DirInfo{}, fmt.Errorf("couldn't get rclone info: %w", err)
	}

	info, err := s.convertRcloneInfo(rcloneInfo, dir)
	if err != nil {
		return DirInfo{}, fmt.Errorf("couldn't convert rclone info: %w", err)
	}

	info.IsNotFound = isNotFound

	return info, nil
}

func (s *Server) convertRcloneInfo(rcloneInfo *rclone.DirInfo, dir string) (DirInfo, error) {
	info := DirInfo{
		BuildInfo: s.cfg.BuildInfo,
		//
		Sort:  rcloneInfo.Sort,
		Order: rcloneInfo.Order,
		Dir:   dir,
		// Always encode entries as a slice.
		Entries: []DirEntry{},
		//
		dirURL: mustParseURL("/"),
	}

	for _, breadcrumb := range rcloneInfo.Breadcrumbs {
		if breadcrumb.Text == "" {
			continue
		}

		// It doesn't make any sense to add another trailing slash (especially, escaped).
		if breadcrumb.Text != "/" {
			info.dirURL = info.dirURL.JoinPath(url.PathEscape(breadcrumb.Text))
		}
		// All directory urls must end with slash.
		info.dirURL = info.dirURL.JoinPath("/")

		text := breadcrumb.Text
		if text == "/" {
			text = "Home"
		}

		uiURL := mustParseURL("/ui").JoinPath(info.dirURL.String()).String()

		info.Breadcrumbs = append(info.Breadcrumbs, DirBreadcrumb{
			Link: uiURL,
			Text: text,
		})
	}

	for _, entry := range rcloneInfo.Entries {
		if entry.Leaf == "" {
			continue
		}

		filename, err := url.PathUnescape(pkgPath.Clean(entry.Leaf))
		if err != nil {
			return DirInfo{}, fmt.Errorf("invalid url %q: %w", entry.Leaf, err)
		}
		filepath := pkgPath.Join(info.Dir, filename)

		var (
			dirURL, webDirURL string
			//
			originalFileURL, thumbnailURL string
			humanReadableSize             string
			fileType                      rview.FileType
			canPreview                    bool
		)
		if entry.IsDir {
			escapedFilename := url.PathEscape(filename)

			dirURL = mustParseURL("/api/dir").JoinPath(info.dirURL.String(), escapedFilename, "/").String()
			webDirURL = mustParseURL("/ui").JoinPath(info.dirURL.String(), escapedFilename, "/").String()

		} else {
			id := rview.NewFileID(filepath, entry.ModTime, entry.Size)

			originalFileURL = fileIDToURL("/api/file", info.dirURL, id)
			humanReadableSize = misc.FormatFileSize(entry.Size)
			fileType = rview.GetFileType(id.GetExt())

			switch fileType {
			case rview.FileTypeText:
				canPreview = true

			case rview.FileTypeImage, rview.FileTypeRawImage:
				switch s.cfg.ImagePreviewMode {
				case rview.ImagePreviewModeNone:
					thumbnailURL = ""
				case rview.ImagePreviewModeOriginal:
					thumbnailURL = originalFileURL
					if fileType == rview.FileTypeRawImage {
						thumbnailURL = ""
					}
				case rview.ImagePreviewModeThumbnails:
					if s.thumbnailService.CanGenerateThumbnail(id) {
						thumbnailURL = fileIDToURL("/api/thumbnail", info.dirURL, id)
					}
				}
				if thumbnailURL != "" {
					canPreview = true
				}

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
			HumanReadableModTime: misc.FormatModTime(modTime),
			FileType:             fileType,
			CanPreview:           canPreview,
			//
			DirURL:          dirURL,
			WebDirURL:       webDirURL,
			OriginalFileURL: originalFileURL,
			ThumbnailURL:    thumbnailURL,
			IconName:        static.GetFileIcon(filename, entry.IsDir),
		})
		if entry.IsDir {
			info.DirCount++
		} else {
			info.FileCount++
			info.TotalFileSize += entry.Size
		}
	}
	return info, nil
}

// handleFile proxies the request to Rclone that knows how to handle 'Range' headers and other nuances.
func (s *Server) handleFile(w http.ResponseWriter, r *http.Request) {
	fileID, err := fileIDFromRequest(r, "/api/file")
	if err != nil {
		writeBadRequestError(w, "invalid file id: %s", err.Error())
		return
	}

	s.rclone.ProxyFileRequest(fileID, w, r)
}

// handleThumbnail returns the thumbnail.
func (s *Server) handleThumbnail(w http.ResponseWriter, r *http.Request) {
	id, err := fileIDFromRequest(r, "/api/thumbnail")
	if err != nil {
		writeBadRequestError(w, "invalid file id: %s", err.Error())
		return
	}

	var size thumbnails.ThumbnailSize
	switch v := r.FormValue("thumbnail_size"); v {
	case "small":
		size = thumbnails.ThumbnailSmall
	case "medium", "":
		size = thumbnails.ThumbnailMedium
	case "large":
		size = thumbnails.ThumbnailLarge
	default:
		writeBadRequestError(w, "invalid thumbnail_size: %q", v)
		return
	}

	rc, contentType, err := s.thumbnailService.OpenThumbnail(r.Context(), id, size)
	if err != nil {
		if errors.Is(err, cache.ErrCacheMiss) {
			writeError(w, http.StatusNotFound, "no thumbnail for %q, size %d, mod time %q", id.GetPath(), id.GetSize(), id.GetModTime())
			return
		}
		writeBadRequestError(w, "couldn't open thumbnail: %s", err)
		return
	}
	defer rc.Close()

	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	// Use mod time as a value for ETag.
	etag := strconv.Itoa(int(id.GetModTime().Unix()))
	setCacheHeaders(w, 30*24*time.Hour, etag)

	io.Copy(w, rc)
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	searchValue := r.FormValue("search")
	minLength := s.searchService.GetMinSearchLength()
	if len([]rune(searchValue)) < minLength {
		writeBadRequestError(w, `minimum "search" length is %d characters`, minLength)
		return
	}
	limit, err := strconv.Atoi(r.FormValue("limit"))
	if err != nil {
		writeBadRequestError(w, "invalid limit value: %s", err)
		return
	}
	isUI := r.FormValue("ui") != ""

	hits, total, err := s.searchService.Search(r.Context(), searchValue, limit)
	if err != nil {
		writeInternalServerError(w, "search failed: %s", err)
		return
	}

	resp := SearchResponse{
		Hits:  make([]SearchHit, 0, len(hits)),
		Total: total,
	}
	for _, hit := range hits {
		var webURL string
		if hit.IsDir {
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

		resp.Hits = append(resp.Hits, SearchHit{
			Path:   hit.Path,
			IsDir:  hit.IsDir,
			Score:  hit.Score,
			WebURL: webURL,
			Icon:   static.GetFileIcon(hit.Path, hit.IsDir),
		})
	}

	if isUI {
		if len(resp.Hits) == 0 {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		s.executeTemplate(w, "search-results.html", resp)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleRefreshIndex(w http.ResponseWriter, r *http.Request) {
	// Index refresh can take a while, and we don't want to interrupt this process.
	ctx := context.WithoutCancel(r.Context())

	err := s.searchService.RefreshIndex(ctx)
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
	query.Set("size", strconv.FormatInt(id.GetSize(), 10))
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

	rawSize := r.FormValue("size")
	size, err := strconv.ParseInt(rawSize, 10, 64)
	if err != nil {
		return rview.FileID{}, fmt.Errorf("invalid size: %w", err)
	}

	return rview.NewFileID(path, modTime, size), nil
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
