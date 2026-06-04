// Package main contains the deployment API entry point.
package main

import (
	"errors"
	"log"
	"net/http"
	"os"

	"github.com/unionai/idd-sandbox/internal/app"
	"github.com/unionai/idd-sandbox/internal/server"
	"github.com/unionai/idd-sandbox/internal/sqlite"
)

func main() {
	addr := envOrDefault("ADDR", ":8080")
	dbPath := envOrDefault("DEPLOYMENT_DB_PATH", "deployments.db")

	store, err := sqlite.Open(dbPath)
	if err != nil {
		log.Fatalf("open deployment store: %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			log.Printf("close deployment store: %v", err)
		}
	}()

	application := app.New(store)
	httpServer := &http.Server{
		Addr:    addr,
		Handler: server.NewHandler(application),
	}

	log.Printf("deployment-api listening on %s", addr)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("serve deployment api: %v", err)
	}
}

func envOrDefault(name string, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
