package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/gin-gonic/gin"
	"github.com/mohsenm4/blockchain-insight/config"
	"github.com/mohsenm4/blockchain-insight/internal/api"
	"github.com/mohsenm4/blockchain-insight/internal/logging"
)

// @title Blockchain Insight API
// @version 1.0
// @BasePath
func main() {
	env := os.Getenv("APP_ENV")
	slog.SetDefault(logging.New(env))

	if env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	cfg, err := config.LoadConfig(".")
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}

	server := api.NewServer(cfg)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := server.StartWatcher(ctx); err != nil {
		slog.Error("start watcher", "err", err)
		os.Exit(1)
	}

	// TODO: Make the server port configurable via environment variable or config file
	port := ":8080"
	slog.Info("server starting", "addr", port, "env", env)
	if err := server.Start(ctx, port); err != nil {
		slog.Error("server exited", "err", err)
		os.Exit(1)
	}
}
