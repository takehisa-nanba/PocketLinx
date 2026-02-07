package api

import (
	"PocketLinx/pkg/container"
	"embed"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type Server struct {
	engine  *container.Engine
	proxies map[string]*portProxy
	mu      sync.Mutex
}

func NewServer(engine *container.Engine) *Server {
	return &Server{
		engine:  engine,
		proxies: make(map[string]*portProxy),
	}
}

//go:embed index.html style.css app.js
//go:embed logo.png
var uiAssets embed.FS

func (s *Server) Start(port int) error {
	http.HandleFunc("/api/containers", s.handleList)
	http.HandleFunc("/api/start", s.handleStart)
	http.HandleFunc("/api/run", s.handleRun)
	http.HandleFunc("/api/stop", s.handleStop)
	http.HandleFunc("/api/remove", s.handleRemove)
	http.HandleFunc("/api/update", s.handleUpdate)
	http.HandleFunc("/api/logs", s.handleLogs)
	http.HandleFunc("/api/version", s.handleVersion)
	http.HandleFunc("/api/images", s.handleImages)
	http.HandleFunc("/api/compose/projects", s.handleComposeProjects)

	http.HandleFunc("/style.css", s.handleAsset("style.css", "text/css"))
	http.HandleFunc("/app.js", s.handleAsset("app.js", "application/javascript"))
	http.HandleFunc("/logo.png", s.handleAsset("logo.png", "image/png"))
	http.HandleFunc("/", s.handleUI)

	// Start a background loop to sync proxies
	go func() {
		for {
			s.syncProxies()
			time.Sleep(5 * time.Second)
		}
	}()

	fmt.Printf("Dashboard available at http://localhost:%d\n", port)
	return http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
}
