package main

import (
    "context"
    "log"
    "os"
    "os/signal"
    "syscall"

    "albedo-checker/internal/bot"
    "albedo-checker/internal/config"
    "albedo-checker/internal/database"
    "albedo-checker/internal/utils"
    "go.uber.org/zap"
)

func main() {
    cfg, err := config.Load()
    if err != nil {
        log.Fatalf("Failed to load config: %v", err)
    }

    logger, err := utils.NewLogger("info")
    if err != nil {
        log.Fatalf("Failed to init logger: %v", err)
    }
    defer logger.Sync()

    db, err := database.New(cfg.DBPath, logger)
    if err != nil {
        logger.Fatal("Failed to init database", zap.Error(err))
    }
    defer db.Close()

    if err := db.Migrate(); err != nil {
        logger.Fatal("Failed to migrate database", zap.Error(err))
    }

    b, err := bot.New(cfg, db, logger)
    if err != nil {
        logger.Fatal("Failed to init bot", zap.Error(err))
    }

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    go b.Start(ctx)

    logger.Info("Albedo Checker started", zap.Int64("admin", cfg.AdminID))

    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit

    logger.Info("Shutting down...")
    cancel()
}
