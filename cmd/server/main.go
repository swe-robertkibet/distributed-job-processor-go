package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"distributed-job-processor/internal/config"
	"distributed-job-processor/internal/server"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load configuration:", err)
	}

	srv, err := server.New(cfg)
	if err != nil {
		log.Fatal("Failed to create server:", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := srv.Start(ctx); err != nil {
			log.Fatal("Server failed to start:", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")
	srv.Shutdown(ctx)
}