package main

import (
	"context"
	"errors"
	"flag"
	"net/url"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/ShoshinNikita/rview/cache"
	"github.com/ShoshinNikita/rview/resizer"
	"github.com/ShoshinNikita/rview/rlog"
	icons "github.com/ShoshinNikita/rview/static/material-icons"
	"github.com/ShoshinNikita/rview/ui"
	"github.com/ShoshinNikita/rview/web"
)

// Flags
var (
	serverPort int
	rcloneURL  flagURL
	dir        string
	debug      bool

	resizedImageMaxAge        time.Duration
	resizedImagesMaxTotalSize int64

	webCacheMaxAge       time.Duration
	webCacheMaxTotalSize int64
)

type flagURL struct {
	URL *url.URL
}

func (u *flagURL) MarshalText() ([]byte, error) {
	if u.URL == nil {
		return nil, nil
	}
	return []byte(u.URL.String()), nil
}

func (u *flagURL) UnmarshalText(text []byte) (err error) {
	if len(text) == 0 {
		return errors.New("url can't be empty")
	}
	u.URL, err = url.Parse(string(text))
	return err
}

func main() {
	flag.IntVar(&serverPort, "port", 8080, "server port")
	flag.TextVar(&rcloneURL, "rclone-url", &flagURL{}, "rclone base url")
	flag.StringVar(&dir, "dir", "./var", "data dir")
	flag.BoolVar(&debug, "debug", false, "enable debug logs")
	//
	flag.DurationVar(&resizedImageMaxAge, "resized-images-max-age", 60*24*time.Hour, "max age of resized images")
	flag.Int64Var(&resizedImagesMaxTotalSize, "resized-images-max-total-size", 200<<20, "max total size of resized images, bytes")
	//
	flag.DurationVar(&webCacheMaxAge, "web-cache-max-age", 60*24*time.Hour, "max age of web cache")
	flag.Int64Var(&webCacheMaxTotalSize, "web-cache-max-total-size", 200<<20, "max total size of web cache, bytes")

	flag.Parse()

	if serverPort == 0 {
		rlog.Fatal("server port must be > 0")
	}
	if rcloneURL.URL == nil {
		rlog.Fatal("rclone base url can't be empty")
	}
	if dir == "" {
		rlog.Fatal("dir can't be empty")
	}

	if debug {
		rlog.EnableDebug()
	}

	if err := icons.Prepare(); err != nil {
		rlog.Fatalf("couldn't prepare icons: %s", err)
	}

	termCtx, termCtxCancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)

	resizerCacheDir := filepath.Join(dir, "thumbnails")
	resizerCache := cache.NewDiskCache(resizerCacheDir)
	resizerCacheCleaner := cache.NewCleaner(resizerCacheDir, resizedImageMaxAge, resizedImagesMaxTotalSize)
	resizer := resizer.NewImageResizer(resizerCache, runtime.NumCPU()+5)

	webCacheDir := filepath.Join(dir, "cache")
	webCache := cache.NewDiskCache(webCacheDir)
	webCacheCleaner := cache.NewCleaner(webCacheDir, webCacheMaxAge, webCacheMaxTotalSize)

	templateFS := ui.New(debug)

	server := web.NewServer(serverPort, rcloneURL.URL, resizer, webCache, templateFS)
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
