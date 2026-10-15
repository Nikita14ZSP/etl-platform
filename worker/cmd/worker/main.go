// Команда worker забирает задачи из очереди и исполняет их.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/Nikita14ZSP/etl-platform/worker/internal/config"
	"github.com/Nikita14ZSP/etl-platform/worker/internal/logging"
	"github.com/Nikita14ZSP/etl-platform/worker/internal/queue"
	"github.com/Nikita14ZSP/etl-platform/worker/internal/runner"
	"github.com/Nikita14ZSP/etl-platform/worker/task"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error:\n%v\n", err)
		os.Exit(1)
	}

	log := logging.New(os.Stdout, cfg.LogFormat, cfg.LogLevel, cfg.WorkerID)

	if err := run(cfg, log); err != nil {
		log.Error("worker failed", "err", err)
		os.Exit(1)
	}
}

func run(cfg config.Config, log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	opts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		return fmt.Errorf("parse REDIS_URL: %w", err)
	}
	rdb := redis.NewClient(opts)
	defer func() { _ = rdb.Close() }()

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		return fmt.Errorf("connect to redis: %w", err)
	}

	consumer := queue.NewConsumer(rdb, cfg.Stream, cfg.ConsumerGroup, cfg.WorkerID, cfg.ReadBlock)
	if err := consumer.EnsureGroup(ctx); err != nil {
		return err
	}

	// TODO: заменить на реестр операторов, когда он появится в operators/.
	// Пока задача только логируется и считается успешной.
	handler := runner.HandlerFunc(func(_ context.Context, msg *task.Message) error {
		// config в лог не пишем: в нём могут оказаться пароли и токены.
		log.Warn("operators are not implemented yet, task skipped", "operator", msg.Operator)
		return nil
	})

	log.Info("worker started", "stream", cfg.Stream, "group", cfg.ConsumerGroup)
	runner.New(consumer, handler, log).Run(ctx)
	log.Info("worker stopped")

	return nil
}
