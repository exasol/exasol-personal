package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gorilla/mux"
)

var dataDir string

func helloHandler(w http.ResponseWriter, r *http.Request) {
	// Read the request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		log.Printf("Error reading request body: %v", err)
		return
	}
	defer r.Body.Close()

	// Create a unique filename based on timestamp
	timestamp := time.Now().Format("20060102-150405.000000")
	filename := fmt.Sprintf("hello-%s.txt", timestamp)
	filepath := filepath.Join(dataDir, filename)

	// Write the content to a file
	err = os.WriteFile(filepath, body, 0644)
	if err != nil {
		http.Error(w, "Failed to write file", http.StatusInternalServerError)
		log.Printf("Error writing file: %v", err)
		return
	}

	log.Printf("Received POST request, wrote %d bytes to %s", len(body), filename)

	// Respond with "hello"
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "hello")
}

func main() {
	// Parse command-line arguments
	flag.StringVar(&dataDir, "dir", "", "Directory to store received data (required)")
	port := flag.Int("port", 8080, "Port to listen on")
	flag.Parse()

	// Validate directory argument
	if dataDir == "" {
		log.Fatal("Error: -dir flag is required")
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		log.Fatalf("Error creating directory %s: %v", dataDir, err)
	}

	// Verify directory is writable
	if info, err := os.Stat(dataDir); err != nil {
		log.Fatalf("Error accessing directory %s: %v", dataDir, err)
	} else if !info.IsDir() {
		log.Fatalf("Error: %s is not a directory", dataDir)
	}

	log.Printf("Starting server on port %d", *port)
	log.Printf("Data directory: %s", dataDir)

	// Setup router
	r := mux.NewRouter()
	r.HandleFunc("/hello", helloHandler).Methods("POST")

	// Start server
	addr := fmt.Sprintf(":%d", *port)
	log.Printf("Server listening on %s", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
