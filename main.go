package main

import (
	"context"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/ShoshinNikita/rview/cache"
	"github.com/ShoshinNikita/rview/config"
	"github.com/ShoshinNikita/rview/rclone"
	"github.com/ShoshinNikita/rview/resizer"
	"github.com/ShoshinNikita/rview/rlog"
	"github.com/ShoshinNikita/rview/rview"
	"github.com/ShoshinNikita/rview/static"
	"github.com/ShoshinNikita/rview/web"
)

func main() {
	cfg, err := config.Parse()
	if err != nil {
		rlog.Fatalf("invalid config: %s", err)
	}

	// TODO: print config?

	config.PrintBuildInfo(cfg.BuildInfo)

	if cfg.DebugLogLevel {
		rlog.EnableDebug()
	}
	if cfg.ReadStaticFilesFromDisk {
		rlog.Info("static files will be read from disk")
	}

	if err := static.Prepare(); err != nil {
		rlog.Fatalf("couldn't prepare icons: %s", err)
	}

	if cfg.Resizer {
		err := resizer.CheckVips()
		if err != nil {
			rlog.Fatal(err)
		}
	}

	// Resizer
	var (
		imageResizer        rview.ImageResizer
		imageResizerCleaner rview.CacheCleaner
	)
	if cfg.Resizer {
		resizerCacheDir := filepath.Join(cfg.Dir, "thumbnails")
		resizerCache, err := cache.NewDiskCache(resizerCacheDir)
		if err != nil {
			rlog.Fatalf("couldn't prepare disk cache for image resizer: %s", err)
		}
		imageResizerCleaner = cache.NewCleaner(resizerCacheDir, cfg.ResizerMaxAge, cfg.ResizerMaxTotalSize)
		imageResizer = resizer.NewImageResizer(resizerCache, cfg.ResizerWorkersCount)
	} else {
		rlog.Info("resizer is disabled")

		imageResizer = resizer.NewNoopImageResizer()
		imageResizerCleaner = cache.NewNoopCleaner()
	}

	// Web Cache
	var (
		webCache        rview.Cache
		webCacheCleaner rview.CacheCleaner
	)
	if cfg.WebCache {
		webCacheDir := filepath.Join(cfg.Dir, "cache")
		webCache, err = cache.NewDiskCache(webCacheDir)
		if err != nil {
			rlog.Fatalf("couldn't prepare disk cache for web: %s", err)
		}
		webCacheCleaner = cache.NewCleaner(webCacheDir, cfg.WebCacheMaxAge, cfg.WebCacheMaxTotalSize)
	} else {
		rlog.Info("web cache is disabled")

		webCache = cache.NewNoopCache()
		webCacheCleaner = cache.NewNoopCleaner()
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
	server := web.NewServer(cfg, imageResizer, webCache)
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
	if err := imageResizer.Shutdown(shutdownCtx); err != nil {
		rlog.Errorf("couldn't shutdown image resizer gracefully: %s", err)
	}
	if err := imageResizerCleaner.Shutdown(shutdownCtx); err != nil {
		rlog.Errorf("couldn't shutdown resizer cache cleaner gracefully: %s", err)
	}
	if err := webCacheCleaner.Shutdown(shutdownCtx); err != nil {
		rlog.Errorf("couldn't shutdown web cache cleaner gracefully: %s", err)
	}
	if err := rcloneInstance.Shutdown(shutdownCtx); err != nil {
		rlog.Errorf("couldn't shutdown rclone instance gracefully: %s", err)
	}
}
