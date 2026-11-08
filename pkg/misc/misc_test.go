package misc

import (
	"testing"

	"github.com/ShoshinNikita/rview/pkg/require"
)

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

func TestFormatSize(t *testing.T) {
	r := require.New(t)

	r.Equal("0B", FormatSize(0))
	r.Equal("1B", FormatSize(1))
	r.Equal("1023B", FormatSize(1023))
	r.Equal("1.0K", FormatSize(1024))
	r.Equal("2.3K", FormatSize(2400))
	r.Equal("1.5M", FormatSize(1548576))
	r.Equal("-1.5M", FormatSize(-1548576))
}
