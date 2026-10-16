package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gsm-api/internal/api"
	"gsm-api/internal/store"
)

func main() {
	logger := log.New(os.Stdout, "gsm-api ", log.LstdFlags|log.LUTC)
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		logger.Fatal("DATABASE_URL is required; copy .env.example and export its values")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	db, err := store.Open(ctx, databaseURL)
	if err != nil {
		logger.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		logger.Fatal(err)
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	config := api.ConfigFromEnv()
	config.LocalOrigin = "http://127.0.0.1:" + port
	service := api.New(db, config, logger)
	go service.RunWebhookWorker(ctx)
	go service.RunSandboxWorker(ctx)
	server := &http.Server{
		Addr:              ":" + port,
		BaseContext:       func(net.Listener) context.Context { return ctx },
		Handler:           service.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Printf("shutdown server: %v", err)
		}
	}()
	logger.Printf("listening on %s", server.Addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Fatal(err)
	}
}
