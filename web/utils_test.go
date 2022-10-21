package web

import (
	"testing"

	"github.com/ShoshinNikita/rview/util/testutil"
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
		testutil.Equal(t, wantRes, got)
	}
}
