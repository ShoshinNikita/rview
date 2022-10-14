package web

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
)

type Server struct {
	httpServer    *http.Server
	rcloneBaseURL *url.URL
}

func NewServer(port int, rcloneBaseURL *url.URL) (s *Server) {
	s = &Server{
		rcloneBaseURL: rcloneBaseURL,
	}

	mux := http.NewServeMux()

	proxy := httputil.NewSingleHostReverseProxy(s.rcloneBaseURL)
	mux.Handle("/file/", http.StripPrefix("/file", proxy))
	mux.HandleFunc("/info", s.handleInfo)
	mux.HandleFunc("/resized/", s.handleResizedImage)

	s.httpServer = &http.Server{
		Addr:    ":" + strconv.Itoa(port),
		Handler: mux,
	}

	return s
}

func (s *Server) Start() error {
	log.Printf("start web server on %q", s.httpServer.Addr)

	err := s.httpServer.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	// TODO
}

func (s *Server) handleResizedImage(w http.ResponseWriter, r *http.Request) {
	// TODO
}
