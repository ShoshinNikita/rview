package rlog

import (
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSetLevel(t *testing.T) {
	r := require.New(t)

	SetLevel(LevelError)
	r.Equal(io.Discard, debug.Writer())
	r.Equal(io.Discard, info.Writer())
	r.Equal(io.Discard, warn.Writer())
	r.Equal(os.Stderr, err.Writer())

	SetLevel(LevelWarn)
	r.Equal(io.Discard, debug.Writer())
	r.Equal(io.Discard, info.Writer())
	r.Equal(os.Stderr, warn.Writer())
	r.Equal(os.Stderr, err.Writer())

	SetLevel(LevelDebug)
	r.Equal(os.Stderr, debug.Writer())
	r.Equal(os.Stderr, info.Writer())
	r.Equal(os.Stderr, warn.Writer())
	r.Equal(os.Stderr, err.Writer())

	SetLevel(LevelInfo)
	r.Equal(io.Discard, debug.Writer()) // should set io.Discard
	r.Equal(os.Stderr, info.Writer())
	r.Equal(os.Stderr, warn.Writer())
	r.Equal(os.Stderr, err.Writer())
}
