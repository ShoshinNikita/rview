package web

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ShoshinNikita/rview/metrics"
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

		// TODO: log errors?

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
	http.ResponseWriter

	statusCode int
}

func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
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
