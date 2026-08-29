package rlog

import (
	"io"
	"os"
	"testing"

	"github.com/ShoshinNikita/rview/pkg/require"
)

func TestSetLevel(t *testing.T) {
	r := require.New(t)

	log := newLogger()

	r.True(os.Stderr == log.debug.Writer())
	r.True(os.Stderr == log.info.Writer())
	r.True(os.Stderr == log.warn.Writer())
	r.True(os.Stderr == log.err.Writer())

	log.SetLevel(LevelError)
	r.True(io.Discard == log.debug.Writer())
	r.True(io.Discard == log.info.Writer())
	r.True(io.Discard == log.warn.Writer())
	r.True(os.Stderr == log.err.Writer())

	log.SetLevel(LevelWarn)
	r.True(io.Discard == log.debug.Writer())
	r.True(io.Discard == log.info.Writer())
	r.True(os.Stderr == log.warn.Writer())
	r.True(os.Stderr == log.err.Writer())

	log.SetLevel(LevelDebug)
	r.True(os.Stderr == log.debug.Writer())
	r.True(os.Stderr == log.info.Writer())
	r.True(os.Stderr == log.warn.Writer())
	r.True(os.Stderr == log.err.Writer())

	log.SetLevel(LevelInfo)
	r.True(io.Discard == log.debug.Writer()) // should set io.Discard
	r.True(os.Stderr == log.info.Writer())
	r.True(os.Stderr == log.warn.Writer())
	r.True(os.Stderr == log.err.Writer())
}
