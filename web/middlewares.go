package web

import (
	"bytes"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ShoshinNikita/rview/pkg/metrics"
	"github.com/ShoshinNikita/rview/pkg/rlog"
	"github.com/prometheus/client_golang/prometheus"
)

func loggingMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/favicon.ico" || strings.HasPrefix(path, "/debug") {
			h.ServeHTTP(w, r)
			return
		}

		prefixes := []string{
			"/static/",
			"/ui/",
			"/api/dir/",
			"/api/file/",
			"/api/thumbnail/",
		}
		for _, prefix := range prefixes {
			if strings.HasPrefix(path, prefix) {
				path = prefix + "*"
				break
			}
		}

		now := time.Now()
		rw := newResponseWriter(w, r)

		h.ServeHTTP(rw, r)

		if !rw.requestCanceledByUser && rw.errMsg.Len() > 0 {
			rlog.Errorf(`request "%s %s" failed with code %d: %s`, r.Method, r.URL.Path, rw.statusCode, rw.errMsg.String())
		}

		statusCode := rw.statusCode
		if rw.requestCanceledByUser {
			statusCode = 499 // 499 Client Closed Request (Nginx)
		}
		metrics.HTTPResponseStatuses.
			With(prometheus.Labels{
				"status": strconv.Itoa(statusCode),
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
	r *http.Request

	statusCode            int
	requestCanceledByUser bool
	headerWritten         bool
	errMsg                *bytes.Buffer
}

func newResponseWriter(w http.ResponseWriter, r *http.Request) *responseWriter {
	return &responseWriter{
		w:      w,
		r:      r,
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
		// Check the context just before writing the header to make sure that request
		// was canceled during its processing, if it was.
		select {
		case <-rw.r.Context().Done():
			rw.requestCanceledByUser = true
		default:
		}

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
