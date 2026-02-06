package api

import (
	"PocketLinx/pkg/container"
	_ "embed" // Required for go:embed
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

//go:embed index.html
var uiHTML string

func (s *Server) Start(port int) error {
	http.HandleFunc("/api/containers", s.handleList)
	http.HandleFunc("/api/stop", s.handleStop)
	http.HandleFunc("/api/remove", s.handleRemove)
	http.HandleFunc("/api/logs", s.handleLogs)
	http.HandleFunc("/api/version", s.handleVersion) // Added
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

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	// For now, hardcoding here to match main.go. ideally should be shared const.
	fmt.Fprintf(w, `{"version": "v0.3.0 (WSL Native Architecture)"}`)
}

func (s *Server) handleUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	if uiHTML == "" {
		fmt.Fprintf(w, "<h1>Error: Dashboard UI not embedded</h1>")
		return
	}
	fmt.Fprintf(w, uiHTML)
}
