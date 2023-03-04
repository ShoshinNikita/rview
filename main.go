package main

import (
	"context"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/ShoshinNikita/rview/cache"
	"github.com/ShoshinNikita/rview/config"
	"github.com/ShoshinNikita/rview/pkg/rlog"
	"github.com/ShoshinNikita/rview/rclone"
	"github.com/ShoshinNikita/rview/rview"
	"github.com/ShoshinNikita/rview/static"
	"github.com/ShoshinNikita/rview/thumbnails"
	"github.com/ShoshinNikita/rview/web"
)

func main() {
	cfg, err := config.Parse()
	if err != nil {
		rlog.Fatalf("invalid config: %s", err)
	}

	config.PrintBuildInfo(cfg.BuildInfo)
	config.PrintConfig(cfg)

	if cfg.DebugLogLevel {
		rlog.EnableDebug()
	}
	if cfg.ReadStaticFilesFromDisk {
		rlog.Info("static files will be read from disk")
	}

	if err := static.Prepare(); err != nil {
		rlog.Fatalf("couldn't prepare icons: %s", err)
	}

	// Thumbnail Service
	var (
		thumbnailService rview.ThumbnailService
		thumbnailCleaner rview.CacheCleaner
	)
	if cfg.Thumbnails {
		err := thumbnails.CheckVips()
		if err != nil {
			rlog.Fatal(err)
		}

		thumbnailsCacheDir := filepath.Join(cfg.Dir, "thumbnails")
		thumbnailsCache, err := cache.NewDiskCache(thumbnailsCacheDir)
		if err != nil {
			rlog.Fatalf("couldn't prepare disk cache for thumbnails: %s", err)
		}
		thumbnailCleaner = cache.NewCleaner(thumbnailsCacheDir, cfg.ThumbnailsMaxAge, cfg.ThumbnailsMaxTotalSize)
		thumbnailService = thumbnails.NewThumbnailService(thumbnailsCache, cfg.ThumbnailsWorkersCount)

	} else {
		rlog.Info("thumbnail service is disabled")

		thumbnailService = thumbnails.NewNoopThumbnailService()
		thumbnailCleaner = cache.NewNoopCleaner()
	}

	termCtx, termCtxCancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)

	// Rclone Instance
	rcloneInstance, err := rclone.NewRclone(cfg.RclonePort, cfg.RcloneTarget)
	if err != nil {
		rlog.Fatalf("couldn't prepare rclone: %s", err)
	}
	go func() {
		if err := rcloneInstance.Start(); err != nil {
			rlog.Errorf("rclone instance error: %s", err)
			termCtxCancel()
		}
	}()

	// Web Server
	server := web.NewServer(cfg, thumbnailService)
	go func() {
		if err := server.Start(); err != nil {
			rlog.Errorf("web server error: %s", err)
			termCtxCancel()
		}
	}()

	<-termCtx.Done()

	rlog.Info("shutdown")

	shutdownCtx, shutdownCtxCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCtxCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		rlog.Errorf("couldn't shutdown web server gracefully: %s", err)
	}
	if err := thumbnailService.Shutdown(shutdownCtx); err != nil {
		rlog.Errorf("couldn't shutdown thumbnail service gracefully: %s", err)
	}
	if err := thumbnailCleaner.Shutdown(shutdownCtx); err != nil {
		rlog.Errorf("couldn't shutdown thumbnail cache cleaner gracefully: %s", err)
	}
	if err := rcloneInstance.Shutdown(shutdownCtx); err != nil {
		rlog.Errorf("couldn't shutdown rclone instance gracefully: %s", err)
	}
}
