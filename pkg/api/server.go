package api

import (
	"PocketLinx/pkg/container"
	"encoding/json"
	"fmt"
	"net/http"
)

type Server struct {
	engine *container.Engine
}

func NewServer(engine *container.Engine) *Server {
	return &Server{engine: engine}
}

func (s *Server) Start(port int) error {
	http.HandleFunc("/api/containers", s.handleList)
	http.HandleFunc("/api/stop", s.handleStop)
	http.HandleFunc("/api/remove", s.handleRemove)
	http.HandleFunc("/api/logs", s.handleLogs)
	http.HandleFunc("/", s.handleUI)

	fmt.Printf("Dashboard available at http://localhost:%d\n", port)
	return http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	containers, err := s.engine.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(containers)
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if err := s.engine.Stop(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "OK")
}

func (s *Server) handleRemove(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if err := s.engine.Remove(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "OK")
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	logs, err := s.engine.Logs(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%s", logs)
}

func (s *Server) handleUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, uiHTML)
}

var uiHTML = "<html><body><h1>PocketLinx Dashboard</h1><p>API is ready.</p></body></html>"
