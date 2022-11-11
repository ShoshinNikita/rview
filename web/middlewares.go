package web

import (
	"bytes"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ShoshinNikita/rview/metrics"
	"github.com/ShoshinNikita/rview/rlog"
	"github.com/prometheus/client_golang/prometheus"
)

func loggingMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case path == "/favicon.ico" || strings.HasPrefix(path, "/debug"):
			h.ServeHTTP(w, r)
			return

		case strings.HasPrefix(path, "/static/"):
			path = "/static/"

		case strings.HasPrefix(path, "/ui/"):
			path = "/ui/"
		}

		now := time.Now()
		rw := newResponseWriter(w)

		h.ServeHTTP(rw, r)

		if rw.errMsg.Len() > 0 {
			rlog.Errorf(`request "%s %s" failed with code %d: %s`, r.Method, r.URL.Path, rw.statusCode, rw.errMsg.String())
		}

		metrics.HTTPResponseStatuses.
			With(prometheus.Labels{
				"status": strconv.Itoa(rw.statusCode),
			}).
			Inc()

		metrics.HTTPResponseTime.
			With(prometheus.Labels{
				"path": path,
			},
			).Observe(time.Since(now).Seconds())
	})
}

type responseWriter struct {
	w http.ResponseWriter

	statusCode    int
	headerWritten bool
	errMsg        *bytes.Buffer
}

func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{
		w:      w,
		errMsg: bytes.NewBuffer(nil),
	}
}

func (rw *responseWriter) Header() http.Header {
	return rw.w.Header()
}

func (rw *responseWriter) Write(data []byte) (int, error) {
	if !rw.headerWritten {
		rw.WriteHeader(http.StatusOK)
	}

	if rw.statusCode >= http.StatusInternalServerError {
		rw.errMsg.Write(data)
	}
	return rw.w.Write(data)
}

func (rw *responseWriter) WriteHeader(code int) {
	if !rw.headerWritten {
		rw.w.WriteHeader(code)
	}

	// Always set status code to handle io.Copy errors.
	rw.statusCode = code
	rw.headerWritten = true
}

// cacheMiddleware sets "Cache-Control" and "Etag" headers.
func cacheMiddleware(maxAge time.Duration, gitHash string, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setCacheHeaders(w, maxAge, gitHash)

		h.ServeHTTP(w, r)
	})
}

func setCacheHeaders(w http.ResponseWriter, maxAge time.Duration, etag string) {
	cacheControl := fmt.Sprintf("private, max-age=%d", int64(maxAge.Seconds()))
	expTime := time.Now().Add(maxAge)

	w.Header().Set("Expires", expTime.Format(http.TimeFormat))
	w.Header().Set("Cache-Control", cacheControl)
	w.Header().Set("ETag", `"`+etag+`"`)
}
