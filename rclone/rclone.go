package rclone

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
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

	httpClient   *http.Client
	rcloneURL    *url.URL
	rcloneTarget string
}

func NewRclone(rclonePort int, rcloneTarget string, dirCacheTime time.Duration) (*Rclone, error) {
	// Check if rclone is installed.
	_, err := exec.LookPath("rclone")
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

	ctx, cancel := context.WithCancel(context.Background())

	//nolint:gosec
	return &Rclone{
		cmd: exec.CommandContext(ctx,
			"rclone",
			"serve",
			"http",
			"--addr", ":"+strconv.Itoa(rclonePort),
			"--template", f.Name(),
			"--dir-cache-time", dirCacheTime.String(),
			rcloneTarget,
		),
		stopCmd:   cancel,
		stoppedCh: make(chan struct{}),
		//
		httpClient: &http.Client{
			Timeout: 2 * time.Minute,
		},
		rcloneURL: &url.URL{
			Scheme: "http",
			Host:   "localhost:" + strconv.Itoa(rclonePort),
		},
		rcloneTarget: rcloneTarget,
	}, nil
}

func (r *Rclone) Start() error {
	defer func() {
		close(r.stoppedCh)
	}()

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
		rlog.Infof("[RCLONE]: %s", s.Text())
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

func (r *Rclone) GetFile(ctx context.Context, id rview.FileID) (io.ReadCloser, http.Header, error) {
	rcloneURL := r.rcloneURL.JoinPath(id.GetPath())

	return r.makRequest(ctx, rcloneURL)
}

func (r *Rclone) GetDirInfo(ctx context.Context, path string, sort, order string) (*rview.RcloneDirInfo, error) {
	now := time.Now()
	defer func() {
		dur := time.Since(now)

		metrics.RcloneResponseTime.Observe(dur.Seconds())
		rlog.Debugf("rclone info for %q was loaded in %s", path, dur)
	}()

	rcloneURL := r.rcloneURL.JoinPath(path)
	rcloneURL.RawQuery = url.Values{
		"sort":  []string{sort},
		"order": []string{order},
	}.Encode()
	body, _, err := r.makRequest(ctx, rcloneURL)
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
	rcloneInfo.Path = html.UnescapeString(rcloneInfo.Path)
	for i := range rcloneInfo.Breadcrumbs {
		rcloneInfo.Breadcrumbs[i].Text = html.UnescapeString(rcloneInfo.Breadcrumbs[i].Text)
	}
	for i := range rcloneInfo.Entries {
		rcloneInfo.Entries[i].URL = html.UnescapeString(rcloneInfo.Entries[i].URL)
	}

	return &rcloneInfo, nil
}

func (r *Rclone) makRequest(ctx context.Context, url *url.URL) (io.ReadCloser, http.Header, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url.String(), nil)
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

func (r *Rclone) GetAllFiles(ctx context.Context) (res []string, err error) {
	//nolint:gosec
	cmd := exec.CommandContext(ctx,
		"rclone",
		"lsf",
		"-R",
		r.rcloneTarget,
	)

	data, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			stderr := string(exitErr.Stderr)
			if len(stderr) > 50 {
				stderr = stderr[:50] + "..."
			}
			err = fmt.Errorf("%s, stderr: %q", exitErr.ProcessState.String(), stderr)
		}
		return nil, fmt.Errorf("command error: %w", err)
	}

	s := bufio.NewScanner(bytes.NewReader(data))
	for s.Scan() {
		res = append(res, s.Text())
	}
	if err := s.Err(); err != nil {
		return nil, fmt.Errorf("couldn't scan rclone output: %w", err)
	}
	return res, nil
}
