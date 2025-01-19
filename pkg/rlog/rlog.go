package rlog

import (
	"io"
	"log"
	"log/slog"
	"os"
)

// Level is just an alias for [slog.Level] because [slog] package provides all the levels we need.
type Level = slog.Level

const (
	LevelDebug Level = slog.LevelDebug
	LevelInfo  Level = slog.LevelInfo
	LevelWarn  Level = slog.LevelWarn
	LevelError Level = slog.LevelError
)

const flags = log.Ldate | log.Ltime | log.Lmsgprefix

var (
	debug = log.New(io.Discard, "[DBG] ", flags)
	info  = log.New(io.Discard, "[INF] ", flags)
	warn  = log.New(io.Discard, "[WRN] ", flags)
	err   = log.New(io.Discard, "[ERR] ", flags)
)

// SetLevel changes the minimal log level.
func SetLevel(newLevel Level) {
	for lvl, l := range map[Level]*log.Logger{
		LevelDebug: debug,
		LevelInfo:  info,
		LevelWarn:  warn,
		LevelError: err,
	} {
		if lvl >= newLevel {
			l.SetOutput(os.Stderr)
		} else {
			l.SetOutput(io.Discard)
		}
	}
}

func Debug(v ...any)                 { debug.Println(v...) }
func Debugf(format string, v ...any) { debug.Printf(format, v...) }

func Info(v ...any)                 { info.Println(v...) }
func Infof(format string, v ...any) { info.Printf(format, v...) }

func Warn(v ...any)                 { warn.Println(v...) }
func Warnf(format string, v ...any) { warn.Printf(format, v...) }

func Error(v ...any)                 { err.Println(v...) }
func Errorf(format string, v ...any) { err.Printf(format, v...) }
