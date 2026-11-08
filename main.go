package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ShoshinNikita/rview/cmd"
	"github.com/ShoshinNikita/rview/rview"
)

func main() {
	cfg, err := rview.ParseConfig()
	if err != nil {
		slog.Error("invalid config", "err", err)
		os.Exit(1)
	}

	switch cfg.LogFormat {
	case rview.LogFormatJSON:
		logOpts := &slog.HandlerOptions{
			Level: cfg.LogLevel,
		}
		slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, logOpts)))
		slog.Info("rview version", "commit_hash", cfg.BuildInfo.ShortGitHash, "commit_time", cfg.BuildInfo.CommitTime)

	default:
		slog.SetLogLoggerLevel(cfg.LogLevel)
		cfg.BuildInfo.Print()
		cfg.Print()
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

		slog.Info("shutdown rview")
		if err := rview.Shutdown(ctx); err != nil {
			slog.Error("shutdown failed", "err", err)
		}

		if startFinished != nil {
			<-startFinished
		}

		slog.Info("exit")
		os.Exit(exitCode)
	}()

	if err := rview.Init(); err != nil {
		slog.Error("initialization failed", "err", err)
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
