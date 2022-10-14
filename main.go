package main

import (
	"context"
	"flag"
	"log"
	"net/url"
	"os/signal"
	"syscall"
	"time"

	"github.com/ShoshinNikita/rview/web"
)

var (
	serverPort int
	rcloneURL  flagURL
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
	u.URL, err = url.Parse(string(text))
	return err
}

func main() {
	flag.IntVar(&serverPort, "port", 8080, "server port")
	flag.TextVar(&rcloneURL, "rclone-url", &flagURL{}, "rclone base url")
	flag.Parse()

	if serverPort == 0 {
		log.Fatalf("server port must be > 0")
	}
	if rcloneURL.URL == nil {
		log.Fatalf("rclone base url can't be empty")
	}

	termCtx, termCtxCancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)

	server := web.NewServer(serverPort, rcloneURL.URL)
	go func() {
		if err := server.Start(); err != nil {
			log.Printf("web server error: %s", err)
			termCtxCancel()
		}
	}()

	<-termCtx.Done()

	log.Println("shutdown")

	shutdownCtx, shutdownCtxCancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer shutdownCtxCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("couldn't shutdown web server gracefully: %s", err)
	}
}
