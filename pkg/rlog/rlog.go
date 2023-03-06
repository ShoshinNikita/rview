package rlog

import (
	"io"
	"log"
	"os"
)

const flags = log.Ldate | log.Ltime | log.Lmsgprefix

var (
	debug = log.New(io.Discard, "[DBG] ", flags)
	info  = log.New(os.Stdout, "[INF] ", flags)
	warn  = log.New(os.Stdout, "[WRN] ", flags)
	err   = log.New(os.Stdout, "[ERR] ", flags)
)

func EnableDebug() {
	debug.SetOutput(os.Stdout)
}

func Debug(v ...any)                 { debug.Println(v...) }
func Debugf(format string, v ...any) { debug.Printf(format, v...) }

func Info(v ...any)                 { info.Println(v...) }
func Infof(format string, v ...any) { info.Printf(format, v...) }

func Warn(v ...any)                 { warn.Println(v...) }
func Warnf(format string, v ...any) { warn.Printf(format, v...) }

func Error(v ...any)                 { err.Println(v...) }
func Errorf(format string, v ...any) { err.Printf(format, v...) }
