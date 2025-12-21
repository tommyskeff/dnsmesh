package health

import (
	"net/http"
	"sync/atomic"

	"github.com/tommyskeff/dnsmesh/internal/logging"
)

type Server struct {
	ready  atomic.Bool
	server *http.Server
}

func NewServer() *Server {
	return &Server{}
}

func (s *Server) SetReady(ready bool) {
	s.ready.Store(ready)
}

func (s *Server) IsReady() bool {
	return s.ready.Load()
}

func (s *Server) Start(addr string) error {
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if s.ready.Load() {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok"))
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("not ready"))
		}
	})

	s.server = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	logging.Info("Starting health server on %s", addr)
	return s.server.ListenAndServe()
}

func (s *Server) Stop() error {
	if s.server != nil {
		return s.server.Close()
	}
	return nil
}
