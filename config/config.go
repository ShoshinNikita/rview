package config

import (
	"errors"
	"flag"
	"runtime/debug"
	"time"
)

type Config struct {
	ServerPort int
	Dir        string
	Debug      bool

	RcloneTarget string
	RclonePort   int

	Resizer             bool
	ResizerMaxAge       time.Duration
	ResizerMaxTotalSize int64

	WebCache             bool
	WebCacheMaxAge       time.Duration
	WebCacheMaxTotalSize int64

	GitHash string
}

func Parse() (cfg Config, err error) {
	flag.IntVar(&cfg.ServerPort, "port", 8080, "server port")
	flag.StringVar(&cfg.Dir, "dir", "./var", "data dir")
	flag.BoolVar(&cfg.Debug, "debug", false, "enable debug logs")
	//
	flag.IntVar(&cfg.RclonePort, "rclone-port", 8181, "port of a rclone instance")
	flag.StringVar(&cfg.RcloneTarget, "rclone-target", "", "rclone target")
	//
	flag.BoolVar(&cfg.Resizer, "resizer", true, "enable or disable image resizer")
	flag.DurationVar(&cfg.ResizerMaxAge, "resizer-max-age", 60*24*time.Hour, "max age of resized images")
	flag.Int64Var(&cfg.ResizerMaxTotalSize, "resizer-max-total-size", 200<<20, "max total size of resized images, bytes")
	//
	flag.BoolVar(&cfg.WebCache, "web-cache", true, "enable or disable web cache")
	flag.DurationVar(&cfg.WebCacheMaxAge, "web-cache-max-age", 60*24*time.Hour, "max age of web cache")
	flag.Int64Var(&cfg.WebCacheMaxTotalSize, "web-cache-max-total-size", 200<<20, "max total size of web cache, bytes")

	flag.Parse()

	if cfg.ServerPort == 0 {
		return cfg, errors.New("server port must be > 0")
	}
	if cfg.RcloneTarget == "" {
		return cfg, errors.New("rclone target can't be empty")
	}
	if cfg.Dir == "" {
		return cfg, errors.New("dir can't be empty")
	}

	cfg.GitHash = readGitHash()

	return cfg, nil
}

func readGitHash() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" {
			return s.Value
		}
	}
	return ""
}
