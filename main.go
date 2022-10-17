package main

import (
	"context"
	"errors"
	"flag"
	"net/url"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/ShoshinNikita/rview/cache"
	"github.com/ShoshinNikita/rview/resizer"
	"github.com/ShoshinNikita/rview/rlog"
	"github.com/ShoshinNikita/rview/web"
)

var (
	serverPort int
	rcloneURL  flagURL
	dir        string
	debug      bool
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

	termCtx, termCtxCancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)

	cache := cache.NewDiskCache(dir)

	resizer := resizer.NewImageResizer(cache, runtime.NumCPU()+5)

	server := web.NewServer(serverPort, rcloneURL.URL, resizer, cache)
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
}
