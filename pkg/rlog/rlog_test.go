package rlog

import (
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSetLevel(t *testing.T) {
	r := require.New(t)

	log := newLogger()

	r.Equal(os.Stderr, log.debug.Writer())
	r.Equal(os.Stderr, log.info.Writer())
	r.Equal(os.Stderr, log.warn.Writer())
	r.Equal(os.Stderr, log.err.Writer())

	log.SetLevel(LevelError)
	r.Equal(io.Discard, log.debug.Writer())
	r.Equal(io.Discard, log.info.Writer())
	r.Equal(io.Discard, log.warn.Writer())
	r.Equal(os.Stderr, log.err.Writer())

	log.SetLevel(LevelWarn)
	r.Equal(io.Discard, log.debug.Writer())
	r.Equal(io.Discard, log.info.Writer())
	r.Equal(os.Stderr, log.warn.Writer())
	r.Equal(os.Stderr, log.err.Writer())

	log.SetLevel(LevelDebug)
	r.Equal(os.Stderr, log.debug.Writer())
	r.Equal(os.Stderr, log.info.Writer())
	r.Equal(os.Stderr, log.warn.Writer())
	r.Equal(os.Stderr, log.err.Writer())

	log.SetLevel(LevelInfo)
	r.Equal(io.Discard, log.debug.Writer()) // should set io.Discard
	r.Equal(os.Stderr, log.info.Writer())
	r.Equal(os.Stderr, log.warn.Writer())
	r.Equal(os.Stderr, log.err.Writer())
}
