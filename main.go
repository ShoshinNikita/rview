package main

import (
	"context"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/ShoshinNikita/rview/cache"
	"github.com/ShoshinNikita/rview/config"
	"github.com/ShoshinNikita/rview/resizer"
	"github.com/ShoshinNikita/rview/rlog"
	"github.com/ShoshinNikita/rview/static"
	"github.com/ShoshinNikita/rview/web"
)

func main() {
	cfg, err := config.Parse()
	if err != nil {
		rlog.Errorf("invalid config: %s", err)
	}

	if cfg.Debug {
		rlog.EnableDebug()

		rlog.Info("debug mode is enabled")
	}

	rlog.Infof("git hash is %q", cfg.GitHash)

	if err := static.Prepare(); err != nil {
		rlog.Fatalf("couldn't prepare icons: %s", err)
	}

	termCtx, termCtxCancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)

	resizerCacheDir := filepath.Join(cfg.Dir, "thumbnails")
	resizerCache := cache.NewDiskCache(resizerCacheDir)
	resizerCacheCleaner := cache.NewCleaner(resizerCacheDir, cfg.ResizedImageMaxAge, cfg.ResizedImagesMaxTotalSize)
	resizer := resizer.NewImageResizer(resizerCache, runtime.NumCPU()+5)

	webCacheDir := filepath.Join(cfg.Dir, "cache")
	webCache := cache.NewDiskCache(webCacheDir)
	webCacheCleaner := cache.NewCleaner(webCacheDir, cfg.WebCacheMaxAge, cfg.WebCacheMaxTotalSize)

	server := web.NewServer(cfg.ServerPort, cfg.GitHash, cfg.Debug, cfg.RcloneURL.URL, resizer, webCache)
	go func() {
		if err := server.Start(); err != nil {
			rlog.Errorf("web server error: %s", err)
			termCtxCancel()
		}
	}()

	<-termCtx.Done()

	rlog.Info("shutdown")

	shutdownCtx, shutdownCtxCancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer shutdownCtxCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		rlog.Errorf("couldn't shutdown web server gracefully: %s", err)
	}
	if err := resizer.Shutdown(shutdownCtx); err != nil {
		rlog.Errorf("couldn't shutdown image resizer gracefully: %s", err)
	}
	if err := resizerCacheCleaner.Shutdown(shutdownCtx); err != nil {
		rlog.Errorf("couldn't shutdown resizer cache cleaner gracefully: %s", err)
	}
	if err := webCacheCleaner.Shutdown(shutdownCtx); err != nil {
		rlog.Errorf("couldn't shutdown web cache cleaner gracefully: %s", err)
	}
}
