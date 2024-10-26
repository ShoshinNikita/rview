package rview

import (
	"encoding"
	"errors"
	"flag"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"runtime/debug"
	"slices"
	"time"
)

type Config struct {
	BuildInfo BuildInfo

	ServerPort int
	Dir        string

	ImagePreviewMode ImagePreviewMode

	ThumbnailsMaxAgeInDays     int
	ThumbnailsMaxTotalSizeInMB int
	ThumbnailsWorkersCount     int

	Rclone RcloneConfig

	// Debug options

	DebugLogLevel           bool
	ReadStaticFilesFromDisk bool
}

type BuildInfo struct {
	ShortGitHash string
	CommitTime   string
}

type RcloneConfig struct {
	URL    string
	Target string
	Port   int
}

type ImagePreviewMode string

const (
	ImagePreviewModeNone       ImagePreviewMode = "none"
	ImagePreviewModeOriginal   ImagePreviewMode = "original"
	ImagePreviewModeThumbnails ImagePreviewMode = "thumbnails"
)

func (m ImagePreviewMode) MarshalText() (text []byte, err error) {
	return []byte(m), nil
}

func (m *ImagePreviewMode) UnmarshalText(text []byte) error {
	*m = ImagePreviewMode(text)

	supportedValues := []ImagePreviewMode{
		ImagePreviewModeNone, ImagePreviewModeOriginal, ImagePreviewModeThumbnails,
	}
	if !slices.Contains(supportedValues, *m) {
		return fmt.Errorf("supported values: %v", supportedValues)
	}
	return nil
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
			p: &cfg.ServerPort, defaultValue: 8080, desc: "Server port",
		},
		"dir": {
			p: &cfg.Dir, defaultValue: "./var", desc: "Directory for app data (thumbnails and etc.)",
		},
		//
		"rclone-url": {
			p: &cfg.Rclone.URL, defaultValue: "", desc: "" +
				"Url of an existing rclone instance, optional. If url is not specified,\n" +
				"a local rclone instance will be launched with the default config file.\n" +
				"Url should include credentials for Basic Auth, e.g., http://user:pass@rclone:80",
		},
		"rclone-target": {
			p: &cfg.Rclone.Target, defaultValue: "", desc: "Rclone target, required",
		},
		//
		"image-preview-mode": {
			p: &cfg.ImagePreviewMode, defaultValue: ImagePreviewModeThumbnails, desc: "" +
				"Available image preview modes:\n" +
				"  - thumbnails: generate thumbnails\n" +
				"  - original: show original images\n" +
				"  - none: don't show preview for images\n",
		},
		//
		"thumbnails-max-age-days": {
			p: &cfg.ThumbnailsMaxAgeInDays, defaultValue: 365, desc: "Max age of thumbnails, days",
		},
		"thumbnails-max-total-size-mb": {
			p: &cfg.ThumbnailsMaxTotalSizeInMB, defaultValue: 500, desc: "Max total size of thumbnails, MiB",
		},
		"thumbnails-workers-count": {
			p: &cfg.ThumbnailsWorkersCount, defaultValue: runtime.NumCPU(), desc: "Number of workers for thumbnail generation",
		},
		//
		"debug-log-level": {
			p: &cfg.DebugLogLevel, defaultValue: false, desc: "Display debug log messages",
		},
		"read-static-files-from-disk": {
			p: &cfg.ReadStaticFilesFromDisk, defaultValue: false, desc: "Read static files directly from disk",
		},
	}
}

func ParseConfig() (Config, error) {
	cfg := Config{
		BuildInfo: readBuildInfo(),
		Rclone: RcloneConfig{
			Port: 8181,
		},
	}

	var printVersion bool
	flag.BoolVar(&printVersion, "version", false, "Print version and exit")

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
		case encoding.TextUnmarshaler:
			flag.TextVar(p, name, params.defaultValue.(encoding.TextMarshaler), params.desc)
		default:
			return Config{}, fmt.Errorf("flag %q has unsupported type: %T", name, p)
		}
	}

	flag.Parse()

	if printVersion {
		cfg.BuildInfo.Print()
		os.Exit(0)
	}

	if cfg.ServerPort == 0 {
		return cfg, errors.New("server port must be > 0")
	}
	if cfg.Rclone.Target == "" {
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

func (info BuildInfo) Print() {
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

func (cfg Config) Print() {
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
	slices.Sort(names)

	fmt.Print("    Config:\n\n")
	for _, name := range names {
		fmt.Printf("        --%-*s = %v\n", maxNameLength, name, reflect.ValueOf(flags[name].p).Elem())
	}
	fmt.Print("\n")
}
