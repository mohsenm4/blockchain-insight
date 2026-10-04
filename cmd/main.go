package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	cfg, err := config.LoadConfig(".")
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("connect to db", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		slog.Error("ping db", "err", err)
		os.Exit(1)
	}
	slog.Info("db connected")

	server := api.NewServer(cfg)

	if err := server.StartWatcher(ctx); err != nil {
		slog.Error("start watcher", "err", err)
		os.Exit(1)
	}

	// TODO: Make the server port configurable via environment variable or config file
	port := ":5050"
	slog.Info("server starting", "addr", port, "env", env)
	if err := server.Start(ctx, port); err != nil {
		slog.Error("server exited", "err", err)
		os.Exit(1)
	}
}
