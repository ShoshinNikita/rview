package misc

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

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

func FormatSize(bytes int64) string {
	sign := ""
	if bytes < 0 {
		sign = "-"
	}

	bytesF := math.Abs(float64(bytes))
	for _, v := range []struct {
		div    float64
		suffix string
	}{
		{1 << 20, "M"},
		{1 << 10, "K"},
	} {
		if bytesF >= v.div {
			return sign + fmt.Sprintf("%.1f", bytesF/v.div) + v.suffix
		}
	}
	return sign + strconv.FormatInt(bytes, 10) + "B"
}
