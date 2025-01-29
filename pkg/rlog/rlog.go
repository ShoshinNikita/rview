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

type logger struct {
	debug *log.Logger
	info  *log.Logger
	warn  *log.Logger
	err   *log.Logger
}

func newLogger() *logger {
	const flags = log.Ldate | log.Ltime | log.Lmsgprefix

	return &logger{
		debug: log.New(os.Stderr, "[DBG] ", flags),
		info:  log.New(os.Stderr, "[INF] ", flags),
		warn:  log.New(os.Stderr, "[WRN] ", flags),
		err:   log.New(os.Stderr, "[ERR] ", flags),
	}
}

// SetLevel changes the minimal log level.
func (l *logger) SetLevel(newLevel Level) {
	for lvl, l := range map[Level]*log.Logger{
		LevelDebug: l.debug,
		LevelInfo:  l.info,
		LevelWarn:  l.warn,
		LevelError: l.err,
	} {
		if lvl >= newLevel {
			l.SetOutput(os.Stderr)
		} else {
			l.SetOutput(io.Discard)
		}
	}
}

var std = newLogger()

func SetLevel(newLevel Level) {
	std.SetLevel(newLevel)
}

func Debug(v ...any)                 { std.debug.Println(v...) }
func Debugf(format string, v ...any) { std.debug.Printf(format, v...) }

func Info(v ...any)                 { std.info.Println(v...) }
func Infof(format string, v ...any) { std.info.Printf(format, v...) }

func Warn(v ...any)                 { std.warn.Println(v...) }
func Warnf(format string, v ...any) { std.warn.Printf(format, v...) }

func Error(v ...any)                 { std.err.Println(v...) }
func Errorf(format string, v ...any) { std.err.Printf(format, v...) }
