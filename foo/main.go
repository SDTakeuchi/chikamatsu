package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	// Create web server on port 8080
	ws := NewWebServer(8080)

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Received shutdown signal")
		if err := ws.Stop(); err != nil {
			log.Printf("Error stopping web server: %v", err)
		}
		os.Exit(0)
	}()

	// Start the server
	if err := ws.Start(); err != nil {
		log.Fatalf("Failed to start web server: %v", err)
	}
}
