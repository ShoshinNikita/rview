package config

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"time"
)

type Config struct {
	BuildInfo

	ServerPort int
	Dir        string

	RcloneTarget string
	RclonePort   int

	Resizer             bool
	ResizerMaxAge       time.Duration
	ResizerMaxTotalSize int64
	ResizerWorkersCount int

	WebCache             bool
	WebCacheMaxAge       time.Duration
	WebCacheMaxTotalSize int64

	// Debug options

	DebugLogLevel           bool
	ReadStaticFilesFromDisk bool
}

type BuildInfo struct {
	ShortGitHash string
	CommitTime   string
}

func Parse() (cfg Config, err error) {
	cfg.BuildInfo = readBuildInfo()

	var printVersion bool
	flag.BoolVar(&printVersion, "version", false, "print version and exit")
	//
	flag.IntVar(&cfg.ServerPort, "port", 8080, "server port")
	flag.StringVar(&cfg.Dir, "dir", "./var", "data dir")
	//
	flag.IntVar(&cfg.RclonePort, "rclone-port", 8181, "port of a rclone instance")
	flag.StringVar(&cfg.RcloneTarget, "rclone-target", "", "rclone target")
	//
	flag.BoolVar(&cfg.Resizer, "resizer", true, "enable or disable image resizer")
	flag.DurationVar(&cfg.ResizerMaxAge, "resizer-max-age", 60*24*time.Hour, "max age of resized images")
	flag.Int64Var(&cfg.ResizerMaxTotalSize, "resizer-max-total-size", 200<<20, "max total size of resized images, bytes")
	flag.IntVar(&cfg.ResizerWorkersCount, "resizer-workers-count", runtime.NumCPU(), "number of image resize workers")
	//
	flag.BoolVar(&cfg.WebCache, "web-cache", true, "enable or disable web cache")
	flag.DurationVar(&cfg.WebCacheMaxAge, "web-cache-max-age", 60*24*time.Hour, "max age of web cache")
	flag.Int64Var(&cfg.WebCacheMaxTotalSize, "web-cache-max-total-size", 200<<20, "max total size of web cache, bytes")
	//
	flag.BoolVar(&cfg.DebugLogLevel, "debug-log-level", false, "display debug log messages")
	flag.BoolVar(&cfg.ReadStaticFilesFromDisk, "read-static-files-from-disk", false, "read static files directly from disk")

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

func readBuildInfo() BuildInfo {
	res := BuildInfo{
		ShortGitHash: "unknown",
		CommitTime:   "unknown",
	}

	info, ok := debug.ReadBuildInfo()
	if !ok {
		return res
	}

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
				res.CommitTime = t.UTC().Format("2006-01-02 15:04:05 UTC")
			}
		}
	}
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

    Commit Hash: %q
    Commit Time: %q

`,
		info.ShortGitHash,
		info.CommitTime,
	)
}
