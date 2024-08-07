package rclone

import (
	"bufio"
	"context"
	"crypto/rand"
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
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ShoshinNikita/rview/pkg/metrics"
	"github.com/ShoshinNikita/rview/pkg/rlog"
	"github.com/ShoshinNikita/rview/rview"
	"github.com/ShoshinNikita/rview/static"
)

// Rclone is an abstraction for an Rclone instance.
type Rclone struct {
	cmd               *exec.Cmd
	stopCmd           func()
	stoppedByShutdown atomic.Bool
	stoppedCh         chan struct{}

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
		_, err = f.WriteString(static.RcloneTemplate)
		if err != nil {
			return nil, fmt.Errorf("couldn't write rclone template file: %w", err)
		}
		if err := f.Close(); err != nil {
			return nil, fmt.Errorf("couldn't close rclone template file: %w", err)
		}

		host := "localhost:" + strconv.Itoa(cfg.Port)

		ctx, cancel := context.WithCancel(context.Background())

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
		stoppedCh: make(chan struct{}),
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

	rlog.Infof("start rclone on %q", r.rcloneURL.String())

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

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.stoppedCh:
		return nil
	}
}

func (r *Rclone) GetFile(ctx context.Context, id rview.FileID) (io.ReadCloser, error) {
	rcloneURL := r.rcloneURL.JoinPath("["+r.rcloneTarget+"]", id.GetPath())

	body, headers, err := r.makeRequest(ctx, "GET", rcloneURL)
	if err != nil {
		return nil, err
	}

	if err := checkLastModified(id, headers); err != nil {
		body.Close()
		return nil, err
	}

	return body, nil
}

func (r *Rclone) ProxyFileRequest(id rview.FileID, w http.ResponseWriter, req *http.Request) {
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
			if r.StatusCode == http.StatusOK {
				return checkLastModified(id, r.Header)
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			http.Error(w, fmt.Sprintf("couldn't proxy file request: %s", err), http.StatusInternalServerError)
		},
	}
	proxy.ServeHTTP(w, req)
}

func checkLastModified(id rview.FileID, fileHeaders http.Header) error {
	fileModTime, err := time.Parse(http.TimeFormat, fileHeaders.Get("Last-Modified"))
	if err != nil {
		return fmt.Errorf("rclone response has invalid Last-Modified header: %w", err)
	}
	if !fileModTime.Equal(id.GetModTime()) {
		return fmt.Errorf("rclone response has different mod time: %q, expected: %q", fileModTime, id.GetModTime())
	}
	return nil
}

func (r *Rclone) GetDirInfo(ctx context.Context, path string, sort, order string) (*rview.RcloneDirInfo, error) {
	now := time.Now()
	defer func() {
		dur := time.Since(now)

		metrics.RcloneResponseTime.Observe(dur.Seconds())
		rlog.Debugf("rclone info for %q was loaded in %s", path, dur)
	}()

	rcloneURL := r.rcloneURL.JoinPath("["+r.rcloneTarget+"]", path)
	rcloneURL.RawQuery = url.Values{
		"sort":  []string{sort},
		"order": []string{order},
	}.Encode()
	body, _, err := r.makeRequest(ctx, "GET", rcloneURL)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	var rcloneInfo rview.RcloneDirInfo
	err = json.NewDecoder(body).Decode(&rcloneInfo)
	if err != nil {
		return nil, fmt.Errorf("couldn't decode rclone response: %w", err)
	}

	// We have to unescape response. It is safe because we will either use it for rendering
	// with Go templates or return it as JSON.
	for i := range rcloneInfo.Breadcrumbs {
		rcloneInfo.Breadcrumbs[i].Text = html.UnescapeString(rcloneInfo.Breadcrumbs[i].Text)
	}
	for i := range rcloneInfo.Entries {
		rcloneInfo.Entries[i].URL = html.UnescapeString(rcloneInfo.Entries[i].URL)
	}

	// Rclone can't accurately report dir size. So, just reset it (and replicate behavior of 'rclone serve').
	for i := range rcloneInfo.Entries {
		if rcloneInfo.Entries[i].IsDir {
			rcloneInfo.Entries[i].Size = 0
		}
	}

	return &rcloneInfo, nil
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

		bodyPrefix := make([]byte, 50)
		n, _ := resp.Body.Read(bodyPrefix)
		bodyPrefix = bodyPrefix[:n]

		return nil, nil, &rview.RcloneError{
			StatusCode: resp.StatusCode,
			BodyPrefix: string(bodyPrefix),
		}
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
