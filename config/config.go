package config

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime/debug"
	"strconv"
	"time"
)

type Config struct {
	BuildInfo

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
}

type BuildInfo struct {
	ShortGitHash string
	BuildTime    string
}

func Parse() (cfg Config, err error) {
	cfg.BuildInfo = readBuildInfo()

	var printVersion bool
	flag.BoolVar(&printVersion, "version", false, "print version and exit")
	//
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

	if printVersion {
		PrintBuildInfo(cfg.BuildInfo)
		os.Exit(0)
	}

	if cfg.ServerPort == 0 {
		return cfg, errors.New("server port must be > 0")
	}
	if cfg.RcloneTarget == "" {
		return cfg, errors.New("rclone target can't be empty")
	}
	if cfg.Dir == "" {
		return cfg, errors.New("dir can't be empty")
	}

	return cfg, nil
}

func readBuildInfo() (res BuildInfo) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return res
	}

	var isDevel bool
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			res.ShortGitHash = s.Value
			if len(res.ShortGitHash) > 7 {
				res.ShortGitHash = res.ShortGitHash[:7]
			}

		case "vcs.time":
			t, err := time.Parse(time.RFC3339, s.Value)
			if err == nil {
				res.BuildTime = t.UTC().Format("2006-01-02 15:04:05 UTC")
			}

		case "vcs.modified":
			isDevel, _ = strconv.ParseBool(s.Value)
		}
	}

	set := func(s *string) {
		switch {
		case isDevel:
			*s = "devel"
		case *s == "":
			*s = "unknown"
		}
	}
	set(&res.ShortGitHash)
	set(&res.BuildTime)

	return res
}

func PrintBuildInfo(info BuildInfo) {
	fmt.Printf(`
     _____          _                 
    |  __ \        (_)                
    | |__) |__   __ _   ___ __      __
    |  _  / \ \ / /| | / _ \\ \ /\ / /
    | | \ \  \ V / | ||  __/ \ V  V / 
    |_|  \_\  \_/  |_| \___|  \_/\_/  

    Commit:     %q
    Build Time: %q

`,
		info.ShortGitHash,
		info.BuildTime,
	)
}
