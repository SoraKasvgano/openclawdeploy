package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"openclawdeploy/server/backend"
)

func main() {
	app, err := backend.New()
	if err != nil {
		log.Fatalf("bootstrap server failed: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server stopped with error: %v", err)
	}
}
