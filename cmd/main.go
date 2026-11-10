package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mohsenm4/blockchain-insight/config"
	"github.com/mohsenm4/blockchain-insight/internal/api"
	"github.com/mohsenm4/blockchain-insight/internal/confirm"
	"github.com/mohsenm4/blockchain-insight/internal/ledger"
	"github.com/mohsenm4/blockchain-insight/internal/logging"
	"github.com/mohsenm4/blockchain-insight/internal/ratelimit"
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

	var limiter api.RateLimiter // nil interface: no rate limit
	if cfg.RedisURL == "" {
		slog.Warn("REDIS_URL not set: POST /send is not rate limited")
	} else {
		rdb, err := ratelimit.NewClient(cfg.RedisURL)
		if err != nil {
			slog.Error("parse REDIS_URL", "err", err)
			os.Exit(1)
		}
		defer func() { _ = rdb.Close() }()

		// A bad URL stops the service, but Redis being down does not:
		// deposits and reads do not need it, and POST /send answers 503
		// until it is back (go-redis reconnects by itself).
		pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		if err := rdb.Ping(pingCtx).Err(); err != nil {
			slog.Warn("redis unreachable: POST /send answers 503 until it is back", "err", err)
		} else {
			slog.Info("redis connected", "send_limit_per_min", cfg.SendLimit)
		}
		cancel()
		limiter = ratelimit.New(rdb, "ratelimit:send:", cfg.SendLimit, time.Minute)
	}

	lg := ledger.New(pool)
	server := api.NewServer(cfg, lg, limiter)

	if err := server.StartWatcher(ctx); err != nil {
		slog.Error("start watcher", "err", err)
		os.Exit(1)
	}

	client, err := ethclient.DialContext(ctx, cfg.RPCURL)
	if err != nil {
		slog.Error("dial rpc for confirmations", "err", err)
		os.Exit(1)
	}
	defer client.Close()
	slog.Info("confirmation tracker starting", "confirmations", cfg.Confirmations)
	go func() {
		if err := confirm.Run(ctx, confirm.NewEthChain(client), lg, cfg.Confirmations, 10*time.Second); err != nil {
			slog.Info("confirmation tracker stopped", "err", err)
		}
	}()
	go func() {
		if err := confirm.RunWithdrawals(ctx, confirm.NewEthChain(client), lg, cfg.Confirmations, 10*time.Second); err != nil {
			slog.Info("withdrawal tracker stopped", "err", err)
		}
	}()
	go func() {
		if err := confirm.RunReversals(ctx, confirm.NewEthChain(client), lg, cfg.Finality, 10*time.Second); err != nil {
			slog.Info("reversal tracker stopped", "err", err)
		}
	}()

	// TODO: Make the server port configurable via environment variable or config file
	port := ":5050"
	slog.Info("server starting", "addr", port, "env", env)
	if err := server.Start(ctx, port); err != nil {
		slog.Error("server exited", "err", err)
		os.Exit(1)
	}
}
