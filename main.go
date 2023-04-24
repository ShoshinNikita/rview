package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ShoshinNikita/rview/cmd"
	"github.com/ShoshinNikita/rview/pkg/rlog"
	"github.com/ShoshinNikita/rview/rview"
)

func main() {
	cfg, err := rview.ParseConfig()
	if err != nil {
		rlog.Errorf("invalid config: %s", err)
		os.Exit(1)
	}

	cfg.BuildInfo.Print()
	cfg.Print()

	if cfg.DebugLogLevel {
		rlog.EnableDebug()
	}

	rview := cmd.NewRview(cfg)

	// Always shutdown service to not keep any external commands running (for example, rclone).
	var (
		exitCode      int
		startFinished <-chan struct{}
	)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		rlog.Info("shutdown")
		if err := rview.Shutdown(ctx); err != nil {
			rlog.Error(err)
		}

		<-startFinished

		os.Exit(exitCode)
	}()

	if err := rview.Prepare(); err != nil {
		rlog.Error(err)
		exitCode = 1
		return
	}

	termCtx, termCtxCancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	startFinished = rview.Start(func() {
		exitCode = 1
		termCtxCancel()
	})

	<-termCtx.Done()
}
