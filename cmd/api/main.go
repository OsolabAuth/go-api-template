package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"example.com/go-api-template/internal/app"
	"example.com/go-api-template/internal/config"
	"example.com/go-api-template/internal/database"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("configuration error", "error", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	startupCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	db, err := database.Open(startupCtx, cfg.DatabaseURL, cfg.DBMaxConns)
	cancel()
	if err != nil {
		logger.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	if err := app.Run(ctx, cfg, db, logger); err != nil {
		logger.Error("server stopped with error", "error", err)
		os.Exit(1)
	}
}
