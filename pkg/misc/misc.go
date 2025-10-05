package misc

import (
	"fmt"
	"strings"
	"time"
)

var sizeSuffixes = [...]string{"B", "KiB", "MiB", "GiB", "TiB", "PiB"}

func FormatFileSize(bytes int64) string {
	size := float64(bytes)

	var suffixIndex int
	for size/1024 > 1 {
		size /= 1024
		suffixIndex++
	}

	res := []rune(fmt.Sprintf("%.2f", size))
	for i := len(res) - 1; i >= 0; i-- {
		if res[i] != '0' {
			if res[i] == '.' {
				i--
			}
			res = res[:i+1]
			break
		}
	}

	return string(res) + " " + sizeSuffixes[suffixIndex]
}

func FormatModTime(modTime time.Time) string {
	return modTime.UTC().Format("2006-01-02 15:04:05") + " UTC"
}

func EnsurePrefix(s, prefix string) string {
	if strings.HasPrefix(s, prefix) {
		return s
	}
	return prefix + s
}

func EnsureSuffix(s, suffix string) string {
	if strings.HasSuffix(s, suffix) {
		return s
	}
	return s + suffix
}
