package web

import (
	"net/http"
	"net/url"
	"strings"
)

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
