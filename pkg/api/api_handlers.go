package api

import (
	"PocketLinx/pkg/compose"
	"PocketLinx/pkg/container"
	"PocketLinx/pkg/version"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	containers, err := s.engine.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(containers)
}

// --- Compose Project Logic ---
type Project struct {
	Name       string                `json:"name"`
	Containers []container.Container `json:"containers"`
}

func (s *Server) handleComposeProjects(w http.ResponseWriter, r *http.Request) {
	containers, _ := s.engine.List()
	projectsMap := make(map[string][]container.Container)

	for _, c := range containers {
		projectName := "default"
		if data, err := os.ReadFile("plx-compose.yml"); err == nil {
			var cfg compose.ComposeConfig
			if err := yaml.Unmarshal(data, &cfg); err == nil {
				if abs, err := filepath.Abs("."); err == nil {
					projectName = filepath.Base(abs)
				}
			}
		}
		projectsMap[projectName] = append(projectsMap[projectName], c)
	}

	var projects []Project
	for name, cs := range projectsMap {
		projects = append(projects, Project{Name: name, Containers: cs})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(projects)
}

func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var opts container.RunOptions
	if err := json.NewDecoder(r.Body).Decode(&opts); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	opts.Detach = true
	if err := s.engine.Run(opts); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "OK")
}

func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	fmt.Printf("[API] START request for %s\n", id)
	if err := s.engine.Start(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "OK")
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	fmt.Printf("[API] STOP request for %s\n", id)
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

func (s *Server) handleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "id required", http.StatusBadRequest)
		return
	}
	var opts container.RunOptions
	if err := json.NewDecoder(r.Body).Decode(&opts); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.engine.Update(id, opts); err != nil {
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

func (s *Server) handleImages(w http.ResponseWriter, r *http.Request) {
	images, err := s.engine.Images()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(images)
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"version": "%s"}`, version.Current)
}

func (s *Server) handleAsset(name, contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := uiAssets.ReadFile(name)
		if err != nil {
			http.Error(w, "Asset not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Write(data)
	}
}

func (s *Server) handleUI(w http.ResponseWriter, r *http.Request) {
	data, err := uiAssets.ReadFile("index.html")
	if err != nil {
		http.Error(w, "UI not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Write(data)
}
