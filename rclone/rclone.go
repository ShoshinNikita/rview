package rclone

import (
	"bufio"
	"cmp"
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"io/fs"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ShoshinNikita/rview/pkg/metrics"
	"github.com/ShoshinNikita/rview/pkg/rlog"
	"github.com/ShoshinNikita/rview/rview"
)

//go:embed rclone.gotmpl
var rcloneTemplate string

type RcloneError struct {
	StatusCode int
	BodyPrefix string
}

func newRcloneError(resp *http.Response) *RcloneError {
	bodyPrefix := make([]byte, 128)
	n, _ := resp.Body.Read(bodyPrefix)
	bodyPrefix = bodyPrefix[:n]

	return &RcloneError{
		StatusCode: resp.StatusCode,
		BodyPrefix: string(bodyPrefix),
	}
}

func (err *RcloneError) Error() string {
	return fmt.Sprintf("unexpected rclone response: status code: %d, body prefix: %q", err.StatusCode, err.BodyPrefix)
}

func IsNotFoundError(err error) bool {
	var rcloneErr *RcloneError
	return errors.As(err, &rcloneErr) && rcloneErr.StatusCode == http.StatusNotFound
}

// Rclone is an abstraction for an Rclone instance.
type Rclone struct {
	cmd               *exec.Cmd
	stopCmd           func()
	stoppedByShutdown atomic.Bool
	stoppedCh         chan struct{}

	dirCache *dirCache

	httpClient *http.Client
	// rcloneURL with username and password for basic auth.
	rcloneURL    *url.URL
	rcloneTarget string
}

func NewRclone(cfg rview.RcloneConfig) (_ *Rclone, err error) {
	var (
		cmd       *exec.Cmd
		stopCmd   func()
		rcloneURL *url.URL
	)
	if cfg.URL != "" {
		// Use an existing rclone instance.
		stopCmd = func() {}
		rcloneURL, err = url.Parse(cfg.URL)
		if err != nil {
			return nil, fmt.Errorf("couldn't parse rclone url %q: %w", cfg.URL, err)
		}

	} else {
		// Run a new rclone instance.
		user := "rview"
		pass, err := newRandomString(10)
		if err != nil {
			return nil, fmt.Errorf("couldn't generate password for rclone rc: %w", err)
		}

		// Check if rclone is installed.
		_, err = exec.LookPath("rclone")
		if err != nil {
			return nil, err
		}

		f, err := os.CreateTemp("", "rview-rclone-template-*")
		if err != nil {
			return nil, fmt.Errorf("couldn't create temp file for rclone template: %w", err)
		}
		_, err = f.WriteString(rcloneTemplate)
		if err != nil {
			return nil, fmt.Errorf("couldn't write rclone template file: %w", err)
		}
		if err := f.Close(); err != nil {
			return nil, fmt.Errorf("couldn't close rclone template file: %w", err)
		}

		host := "localhost:" + strconv.Itoa(cfg.Port)

		ctx, cancel := context.WithCancel(context.Background())

		//nolint:gosec
		cmd = exec.CommandContext(ctx,
			"rclone",
			"rcd",
			"--rc-user", user,
			"--rc-pass", pass,
			"--rc-serve",
			"--rc-addr", host,
			"--rc-template", f.Name(),
			"--rc-web-gui-no-open-browser",
		)
		stopCmd = cancel
		rcloneURL = &url.URL{
			Scheme: "http",
			Host:   host,
			User:   url.UserPassword(user, pass),
		}
	}

	return &Rclone{
		cmd:       cmd,
		stopCmd:   stopCmd,
		stoppedCh: nil, // created in Start
		//
		dirCache: newDirCache(cfg.DirCacheTTL),
		//
		httpClient: &http.Client{
			Timeout: 2 * time.Minute,
		},
		rcloneURL:    rcloneURL,
		rcloneTarget: cfg.Target,
	}, nil
}

func newRandomString(size int) (string, error) {
	data := make([]byte, size/2)
	_, err := rand.Read(data)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}

func (r *Rclone) Start() error {
	r.stoppedCh = make(chan struct{})
	defer func() {
		close(r.stoppedCh)
	}()

	if r.cmd == nil {
		// We use an existing rclone instance.
		return nil
	}

	stdout, err := r.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("couldn't get rclone stdout: %w", err)
	}
	stderr, err := r.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("couldn't get rclone stderr: %w", err)
	}
	pipes := []io.ReadCloser{stdout, stderr}

	rlog.Infof("start rclone on %q", stripPassword(r.rcloneURL))

	err = r.cmd.Start()
	if err != nil {
		return fmt.Errorf("couldn't start rclone: %w", err)
	}

	var wg sync.WaitGroup
	for _, pipe := range pipes {
		pipe := pipe

		wg.Add(1)
		go func() {
			defer wg.Done()
			r.redirectRcloneLogs(pipe)
		}()
	}

	err = r.cmd.Wait()
	if r.stoppedByShutdown.Load() {
		// Don't return errors like "signal: interrupt".
		err = nil
	}

	// Close just in case.
	for _, pipe := range pipes {
		pipe.Close()
	}

	wg.Wait()

	return err
}

// Copied from het/http/client.go.
func stripPassword(u *url.URL) string {
	_, passSet := u.User.Password()
	if passSet {
		return strings.Replace(u.String(), u.User.String()+"@", u.User.Username()+":***@", 1)
	}
	return u.String()
}

func (r *Rclone) redirectRcloneLogs(pipe io.Reader) {
	s := bufio.NewScanner(pipe)
	for s.Scan() {
		text := s.Text()

		// Skip errors caused by request cancellation.
		if strings.Contains(text, "Didn't finish writing GET request") || strings.Contains(text, "context canceled") {
			continue
		}

		rlog.Infof("[RCLONE]: %s", text)
	}
	if err := s.Err(); err != nil && !errors.Is(err, fs.ErrClosed) {
		rlog.Errorf("couldn't read rclone logs: %s", err)
	}
}

func (r *Rclone) Shutdown(ctx context.Context) error {
	r.stoppedByShutdown.Store(true)
	r.stopCmd()

	if r.stoppedCh == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.stoppedCh:
		return nil
	}
}

// OpenFile returns a file content in the form of [io.ReadCloser]. This method should be used only
// for internal purposes (for example, to download image for resizing). Serving files to users
// should be done via [Rclone.ProxyFileRequest].
func (r *Rclone) OpenFile(ctx context.Context, id rview.FileID) (io.ReadCloser, error) {
	rcloneURL := r.rcloneURL.JoinPath("["+r.rcloneTarget+"]", id.GetPath())

	now := time.Now()
	body, headers, err := r.makeRequest(ctx, "GET", rcloneURL)
	if err != nil {
		return nil, err
	}
	metrics.RcloneGetFileHeadersDuration.Observe(time.Since(now).Seconds())

	if err := checkModTime(id, headers); err != nil {
		body.Close()
		return nil, err
	}
	if err := checkContentLength(id, headers); err != nil {
		body.Close()
		return nil, err
	}
	return body, nil
}

func (r *Rclone) RequestFileRange(ctx context.Context, id rview.FileID, rangeStart, rangeEnd int) (io.ReadCloser, error) {
	rcloneURL := r.rcloneURL.JoinPath("["+r.rcloneTarget+"]", id.GetPath())
	req, err := http.NewRequestWithContext(ctx, "GET", rcloneURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("couldn't prepare request: %w", err)
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", rangeStart, rangeEnd))

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode != http.StatusPartialContent {
		defer resp.Body.Close()

		return nil, newRcloneError(resp)
	}
	if err := checkModTime(id, resp.Header); err != nil {
		resp.Body.Close()
		return nil, err
	}
	if err := checkContentRange(id, resp.Header); err != nil {
		resp.Body.Close()
		return nil, err
	}
	return resp.Body, nil
}

func (r *Rclone) ProxyFileRequest(id rview.FileID, w http.ResponseWriter, req *http.Request) {
	now := time.Now()

	proxy := httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			u := r.rcloneURL.JoinPath("["+r.rcloneTarget+"]", id.GetPath())

			// Rclone requires leading slash.
			if !strings.HasPrefix(u.Path, "/") {
				u.Path = "/" + u.Path
			}

			// Basic Auth should be passed via headers.
			if u.User != nil {
				user := u.User.Username()
				pass, _ := u.User.Password()
				pr.Out.SetBasicAuth(user, pass)
			}

			u.User = nil
			pr.Out.URL = u
		},
		ModifyResponse: func(r *http.Response) error {
			if r.StatusCode != http.StatusOK {
				return nil
			}

			metrics.RcloneGetFileHeadersDuration.Observe(time.Since(now).Seconds())

			if err := checkModTime(id, r.Header); err != nil {
				return err
			}
			if err := checkContentLength(id, r.Header); err != nil {
				return err
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			http.Error(w, fmt.Sprintf("couldn't proxy file request: %s", err), http.StatusInternalServerError)
		},
	}
	proxy.ServeHTTP(w, req)
}

func checkModTime(id rview.FileID, fileHeaders http.Header) error {
	fileModTime, err := time.Parse(http.TimeFormat, fileHeaders.Get("Last-Modified"))
	if err != nil {
		return fmt.Errorf("rclone response has invalid Last-Modified header: %w", err)
	}
	if !fileModTime.Equal(id.GetModTime()) {
		return fmt.Errorf("rclone response has different mod time: %q, expected: %q", fileModTime, id.GetModTime())
	}
	return nil
}

func checkContentLength(id rview.FileID, fileHeaders http.Header) error {
	size, err := strconv.Atoi(fileHeaders.Get("Content-Length"))
	if err != nil {
		return fmt.Errorf("rclone response has invalid Content-Length header: %w", err)
	}
	if int64(size) != id.GetSize() {
		return fmt.Errorf("rclone response has different size: %d, expected: %d", size, id.GetSize())
	}
	return nil
}

func checkContentRange(id rview.FileID, fileHeaders http.Header) error {
	contentRange := fileHeaders.Get("Content-Range")
	_, rawSize, _ := strings.Cut(contentRange, "/")
	size, err := strconv.Atoi(rawSize)
	if err != nil {
		return fmt.Errorf("rclone response has invalid size in Content-Range header: %q: %w", contentRange, err)
	}
	if int64(size) != id.GetSize() {
		return fmt.Errorf("rclone response has different size: %d, expected: %d", size, id.GetSize())
	}
	return nil
}

type DirInfo struct {
	Sort  string `json:"sort"`
	Order string `json:"order"`

	Breadcrumbs []DirBreadcrumb `json:"breadcrumbs"`
	Entries     []DirEntry      `json:"entries"`
}

type DirBreadcrumb struct {
	Text string `json:"text"`
}

type DirEntry struct {
	Leaf    string `json:"leaf"`
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"mod_time"`
}

func (r *Rclone) GetDirInfo(ctx context.Context, path string, sort, order string) (*DirInfo, error) {
	info, err := r.getDirInfo(ctx, path)
	if err != nil {
		return nil, err
	}

	sortFn := map[string]func(a DirEntry, b DirEntry) int{
		"":             sortByName,
		"namedirfirst": sortByName,
		"size":         sortBySize,
		"time":         sortByTime,
	}[sort]
	if sortFn != nil {
		info.Sort = sort
		slices.SortFunc(info.Entries, sortFn)
	}

	reverse, ok := map[string]bool{
		"":     false,
		"asc":  false,
		"desc": true,
	}[order]
	if ok {
		info.Order = order
		if reverse {
			slices.Reverse(info.Entries)
		}
	}

	return info, nil
}

func (r *Rclone) getDirInfo(ctx context.Context, path string) (*DirInfo, error) {
	saveToCache := func(*DirInfo) error { return nil }
	if r.dirCache.Enabled() {
		cacheItem := r.dirCache.Get(path)

		// Prevent parallel requests for the same dir.
		cacheItem.Lock()
		defer cacheItem.Unlock()

		info, err := cacheItem.LoadLocked()
		if err != nil {
			return nil, fmt.Errorf("couldn't load dir from cache: %w", err)
		}
		if info != nil {
			metrics.RcloneDirsServedFromCache.Inc()
			return info, nil
		}

		// No data in cache - proceed to load the directory.

		saveToCache = cacheItem.StoreLocked
	}

	now := time.Now()

	rcloneURL := r.rcloneURL.JoinPath("["+r.rcloneTarget+"]", path)
	body, _, err := r.makeRequest(ctx, "GET", rcloneURL)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	var rcloneInfo *DirInfo
	err = json.NewDecoder(body).Decode(&rcloneInfo)
	if err != nil {
		return nil, fmt.Errorf("couldn't decode rclone response: %w", err)
	}

	dur := time.Since(now)
	metrics.RcloneGetDirInfoDuration.Observe(dur.Seconds())
	rlog.Debugf("rclone info for %q was loaded in %s", path, dur)

	// We have to unescape response. It is safe because we will either use it for rendering
	// with Go templates or return it as JSON.
	for i := range rcloneInfo.Breadcrumbs {
		rcloneInfo.Breadcrumbs[i].Text = html.UnescapeString(rcloneInfo.Breadcrumbs[i].Text)
	}
	for i := range rcloneInfo.Entries {
		rcloneInfo.Entries[i].Leaf = html.UnescapeString(rcloneInfo.Entries[i].Leaf)
	}

	// Rclone can't accurately report dir size. So, just reset it (and replicate behavior of 'rclone serve').
	for i := range rcloneInfo.Entries {
		if rcloneInfo.Entries[i].IsDir {
			rcloneInfo.Entries[i].Size = 0
		}
	}

	if err := saveToCache(rcloneInfo); err != nil {
		return nil, fmt.Errorf("couldn't save dir to cache: %w", err)
	}

	return rcloneInfo, nil
}

func sortByName(a, b DirEntry) int {
	if a.IsDir == b.IsDir {
		return cmp.Compare(strings.ToLower(a.Leaf), strings.ToLower(b.Leaf))
	}
	if a.IsDir {
		return -1
	}
	return +1
}

func sortBySize(a, b DirEntry) int {
	switch {
	case (a.IsDir && b.IsDir) || (a.Size == b.Size):
		return cmp.Compare(strings.ToLower(a.Leaf), strings.ToLower(b.Leaf))
	case a.IsDir:
		return -1
	case b.IsDir:
		return +1
	default:
		return cmp.Compare(a.Size, b.Size)
	}
}

func sortByTime(a, b DirEntry) int {
	if a.ModTime != b.ModTime {
		return cmp.Compare(a.ModTime, b.ModTime)
	}
	return sortByName(a, b)
}

func (r *Rclone) makeRequest(ctx context.Context, method string, url *url.URL) (io.ReadCloser, http.Header, error) {
	req, err := http.NewRequestWithContext(ctx, method, url.String(), nil)
	if err != nil {
		return nil, nil, fmt.Errorf("couldn't prepare request: %w", err)
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()

		return nil, nil, newRcloneError(resp)
	}

	return resp.Body, resp.Header, nil
}

func (r *Rclone) GetAllFiles(ctx context.Context) (dirs, files []string, err error) {
	// Pass parameters as a query instead of JSON to be able to forbid access to
	// other remotes via Nginx (see 'docs/advanced_setup.md').

	opt, _ := json.Marshal(map[string]any{ // error is always nil
		"noModTime":  true,
		"noMimeType": true,
		"recurse":    true,
	})
	query := url.Values{
		"fs":     {r.rcloneTarget},
		"remote": {""},
		"opt":    {string(opt)},
	}
	url := r.rcloneURL.JoinPath("operations/list")
	url.RawQuery = query.Encode()

	body, _, err := r.makeRequest(ctx, "POST", url)
	if err != nil {
		return nil, nil, err
	}
	defer body.Close()

	var resp struct {
		List []struct {
			Path  string
			IsDir bool
		} `json:"list"`
	}
	err = json.NewDecoder(body).Decode(&resp)
	if err != nil {
		return nil, nil, fmt.Errorf("couldn't decode rclone response: %w", err)
	}

	for _, v := range resp.List {
		if v.IsDir {
			dirs = append(dirs, v.Path)
		} else {
			files = append(files, v.Path)
		}
	}
	return dirs, files, nil
}
