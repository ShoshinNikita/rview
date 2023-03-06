package rclone

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/ShoshinNikita/rview/pkg/rlog"
	"github.com/ShoshinNikita/rview/static"
)

// Rclone is an abstraction for an Rclone instance.
type Rclone struct {
	cmd               *exec.Cmd
	stopCmd           func()
	stoppedByShutdown atomic.Bool
	stoppedCh         chan struct{}
}

func NewRclone(rclonePort int, rcloneTarget string) (*Rclone, error) {
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
			rcloneTarget,
		),
		stopCmd:   cancel,
		stoppedCh: make(chan struct{}),
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
