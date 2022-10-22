package web

import (
	"fmt"
	"net/http"
	"time"
)

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
