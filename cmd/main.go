package main

import (
	"log/slog"
	"os"

	"github.com/Mohsen20031203/blockchain-insight/config"
	"github.com/Mohsen20031203/blockchain-insight/internal/api"
	"github.com/Mohsen20031203/blockchain-insight/internal/logging"
	"github.com/gin-gonic/gin"
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

	slog.Info("server starting", "addr", ":5050", "env", env)
	if err := server.Start(":5050"); err != nil {
		slog.Error("server exited", "err", err)
		os.Exit(1)
	}
}
