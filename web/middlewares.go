package web

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// cacheMiddleware sets "Cache-Control" and "Etag" headers.
func cacheMiddleware(maxAge time.Duration, gitHash string, h http.Handler) http.Handler {
	cacheControl := fmt.Sprintf("private, max-age=%d", int64(maxAge.Seconds()))
	etag := fmt.Sprintf(`"%s"`, gitHash)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expTime := time.Now().Add(maxAge)

		w.Header().Set("Expires", expTime.Format(http.TimeFormat))
		w.Header().Set("Cache-Control", cacheControl)
		w.Header().Set("ETag", etag)

		h.ServeHTTP(w, r)
	})
}

func replacePath(old, new string, n int, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.Replace(r.URL.Path, old, new, n)
		rawPath := strings.Replace(r.URL.RawPath, old, new, n)

		r2 := &http.Request{}
		*r2 = *r
		r2.URL = &url.URL{}
		*r2.URL = *r.URL
		r2.URL.Path = path
		r2.URL.RawPath = rawPath
		h.ServeHTTP(w, r2)
	})
}
