package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

type HealthResponse struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Uptime    string    `json:"uptime"`
}

type WebServer struct {
	server    *http.Server
	startTime time.Time
}

func NewWebServer(port int) *WebServer {
	mux := http.NewServeMux()

	ws := &WebServer{
		server: &http.Server{
			Addr:    fmt.Sprintf(":%d", port),
			Handler: mux,
		},
		startTime: time.Now(),
	}

	// Register health check endpoint
	mux.HandleFunc("/health", ws.healthHandler)

	return ws
}

func (ws *WebServer) healthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	log.Printf("health check")
	log.Printf("ip: %s", r.RemoteAddr)

	uptime := time.Since(ws.startTime)
	response := HealthResponse{
		Status:    "healthy",
		Timestamp: time.Now(),
		Uptime:    uptime.String(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding health response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (ws *WebServer) Start() error {
	log.Printf("Starting web server on %s", ws.server.Addr)
	return ws.server.ListenAndServe()
}

func (ws *WebServer) Stop() error {
	log.Println("Stopping web server...")
	return ws.server.Close()
}
