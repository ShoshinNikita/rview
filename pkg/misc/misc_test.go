package misc

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFormatFileSize(t *testing.T) {
	for size, wantRes := range map[int64]string{
		8:                     "8 B",
		1 << 15:               "32 KiB",
		1 << 20:               "1024 KiB",
		3 << 20:               "3 MiB",
		3<<20 + 1<<19:         "3.5 MiB",
		3<<20 + 1<<19 + 1<<18: "3.75 MiB",
		2 << 30:               "2 GiB",
	} {
		got := FormatFileSize(size)
		require.Equal(t, wantRes, got)
	}
}

func TestEnsurePrefix(t *testing.T) {
	r := require.New(t)
	r.Equal("/hello", EnsurePrefix("hello", "/"))
	r.Equal("/hello/", EnsurePrefix("hello/", "/"))
	r.Equal("/hello", EnsurePrefix("/hello", "/"))
	r.Equal("x/hello/", EnsurePrefix("/hello/", "x"))
}

func TestEnsureSuffix(t *testing.T) {
	r := require.New(t)
	r.Equal("hello/", EnsureSuffix("hello", "/"))
	r.Equal("/hello/", EnsureSuffix("/hello", "/"))
	r.Equal("/hello/", EnsureSuffix("/hello/", "/"))
	r.Equal("/hello/x", EnsureSuffix("/hello/", "x"))
}
