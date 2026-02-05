// Package main runs the background job worker (recording upload to S3, etc.).
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/aura-webinar/backend/config"
	"github.com/aura-webinar/backend/internal/recordings"
	"github.com/aura-webinar/backend/internal/worker"
	"github.com/aura-webinar/backend/pkg/database"
	"github.com/aura-webinar/backend/pkg/queue"
	"github.com/aura-webinar/backend/pkg/redis"
	"github.com/aura-webinar/backend/pkg/storage"
)

func main() {
	logger := newLogger()
	defer logger.Sync()

	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("load config", zap.Error(err))
	}

	ctx := context.Background()
	pool, err := database.NewPostgresPool(ctx, cfg.Database.DSN(), logger)
	if err != nil {
		logger.Fatal("database", zap.Error(err))
	}
	defer pool.Close()

	rdb, err := redis.NewClient(ctx, cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB, logger)
	if err != nil {
		logger.Fatal("redis", zap.Error(err))
	}
	defer rdb.Close()

	s3Cfg := storage.S3Config{
		Region:               cfg.AWS.Region,
		AccessKeyID:          cfg.AWS.AccessKeyID,
		SecretAccessKey:      cfg.AWS.SecretAccessKey,
		AdsBucket:            cfg.AWS.AdsBucket,
		RecordingsBucket:     cfg.AWS.RecordingsBucket,
		PresignExpireMinutes: cfg.AWS.PresignExpireMinutes,
	}
	s3Client, err := storage.NewS3(ctx, s3Cfg, logger)
	if err != nil {
		logger.Fatal("s3", zap.Error(err))
	}

	recRepo := recordings.NewRepository(pool)
	jobQueue := queue.NewQueue(rdb.Client, logger)
	processor := worker.NewRecordingProcessor(recRepo, s3Client, jobQueue, logger)

	workerCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go processor.Run(workerCtx)
	logger.Info("worker started")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	cancel()
	time.Sleep(2 * time.Second)
	logger.Info("worker stopped")
}

func newLogger() *zap.Logger {
	config := zap.NewProductionConfig()
	config.EncoderConfig.TimeKey = "timestamp"
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	logger, _ := config.Build()
	return logger
}
