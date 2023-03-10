package config

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"runtime/debug"
	"sort"
	"time"
)

type Config struct {
	BuildInfo

	ServerPort int
	Dir        string

	RcloneTarget string
	RclonePort   int

	Thumbnails                 bool
	ThumbnailsMaxAgeInDays     int
	ThumbnailsMaxTotalSizeInMB int
	ThumbnailsWorkersCount     int

	// Debug options

	DebugLogLevel           bool
	ReadStaticFilesFromDisk bool
}

type BuildInfo struct {
	ShortGitHash string
	CommitTime   string
}

type flagParams struct {
	// p is a pointer to a value.
	p            any
	defaultValue any
	desc         string
}

func (cfg *Config) getFlagParams() map[string]flagParams {
	return map[string]flagParams{
		"port": {
			p: &cfg.ServerPort, defaultValue: 8080, desc: "server port",
		},
		"dir": {
			p: &cfg.Dir, defaultValue: "./var", desc: "directory for app data",
		},
		//
		"rclone-port": {
			p: &cfg.RclonePort, defaultValue: 8181, desc: "port of a rclone instance",
		},
		"rclone-target": {
			p: &cfg.RcloneTarget, defaultValue: "", desc: "rclone target",
		},
		//
		"thumbnails": {
			p: &cfg.Thumbnails, defaultValue: true, desc: "generate image thumbnails",
		},
		"thumbnails-max-age-days": {
			p: &cfg.ThumbnailsMaxAgeInDays, defaultValue: 365, desc: "max age of thumbnails, days",
		},
		"thumbnails-max-total-size-mb": {
			p: &cfg.ThumbnailsMaxTotalSizeInMB, defaultValue: 500, desc: "max total size of thumbnails, MiB",
		},
		"thumbnails-workers-count": {
			p: &cfg.ThumbnailsWorkersCount, defaultValue: runtime.NumCPU(), desc: "number of workers for thumbnail generation",
		},
		//
		"debug-log-level": {
			p: &cfg.DebugLogLevel, defaultValue: false, desc: "display debug log messages",
		},
		"read-static-files-from-disk": {
			p: &cfg.ReadStaticFilesFromDisk, defaultValue: false, desc: "read static files directly from disk",
		},
	}
}

func Parse() (cfg Config, err error) {
	cfg.BuildInfo = readBuildInfo()

	var printVersion bool
	flag.BoolVar(&printVersion, "version", false, "print version and exit")

	flags := cfg.getFlagParams()
	for name, params := range flags {
		switch p := params.p.(type) {
		case *bool:
			flag.BoolVar(p, name, params.defaultValue.(bool), params.desc)
		case *int:
			flag.IntVar(p, name, params.defaultValue.(int), params.desc)
		case *int64:
			flag.Int64Var(p, name, params.defaultValue.(int64), params.desc)
		case *string:
			flag.StringVar(p, name, params.defaultValue.(string), params.desc)
		}
	}

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

    GitHub Repo: https://github.com/ShoshinNikita/rview

`,
		info.ShortGitHash,
		info.CommitTime,
	)
}

func PrintConfig(cfg Config) {
	flags := cfg.getFlagParams()

	var (
		names         = make([]string, 0, len(flags))
		maxNameLength int
	)
	for name := range flags {
		if len(name) > maxNameLength {
			maxNameLength = len(name)
		}
		names = append(names, name)
	}
	sort.Strings(names)

	fmt.Print("    Config:\n\n")
	for _, name := range names {
		fmt.Printf("        --%-*s = %v\n", maxNameLength, name, reflect.ValueOf(flags[name].p).Elem())
	}
	fmt.Print("\n")
}
