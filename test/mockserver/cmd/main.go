// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package main provides the entry point for the mock Cloudflare API server.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/StringKe/cloudflare-operator/test/mockserver"
)

func main() {
	port := flag.Int("port", 8787, "Port to listen on")
	flag.Parse()

	server := mockserver.NewServer(mockserver.WithPort(*port))

	// Handle shutdown gracefully
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutting down server...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Stop(ctx); err != nil {
			log.Printf("Error shutting down server: %v", err)
		}
	}()

	log.Printf("Starting mock Cloudflare API server on port %d", *port)
	log.Printf("Health check: http://localhost:%d/health", *port)
	log.Printf("Admin reset: POST http://localhost:%d/admin/reset", *port)

	if err := server.Start(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
