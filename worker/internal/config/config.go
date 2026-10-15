// Package config читает настройки воркера из переменных окружения.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config — настройки воркера. Значения по умолчанию заданы в Load.
type Config struct {
	WorkerID string

	RedisURL      string
	Stream        string
	ConsumerGroup string
	// Сколько ждать новое сообщение в XREADGROUP, прежде чем повторить запрос.
	ReadBlock time.Duration

	// Понадобится для записи статусов задач в Postgres.
	PostgresDSN string

	Concurrency     int
	ShutdownTimeout time.Duration

	LogLevel  slog.Level
	LogFormat string // json или text
}

// Load читает конфигурацию из окружения процесса.
func Load() (Config, error) {
	return load(os.LookupEnv)
}

func load(lookup func(string) (string, bool)) (Config, error) {
	get := func(key, def string) string {
		if v, ok := lookup(key); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
		return def
	}

	cfg := Config{
		WorkerID:      get("WORKER_ID", defaultWorkerID()),
		RedisURL:      get("REDIS_URL", ""),
		Stream:        get("TASK_STREAM", "etl:tasks"),
		ConsumerGroup: get("CONSUMER_GROUP", "etl-workers"),
		PostgresDSN:   get("PG_DSN", ""),
		LogFormat:     get("LOG_FORMAT", "json"),
	}

	var errs []error

	if cfg.RedisURL == "" {
		errs = append(errs, errors.New("REDIS_URL is required"))
	}

	var err error
	if cfg.Concurrency, err = strconv.Atoi(get("CONCURRENCY", "1")); err != nil || cfg.Concurrency < 1 {
		errs = append(errs, fmt.Errorf("CONCURRENCY must be a positive integer, got %q", get("CONCURRENCY", "")))
	}
	if cfg.ReadBlock, err = time.ParseDuration(get("READ_BLOCK", "5s")); err != nil || cfg.ReadBlock <= 0 {
		errs = append(errs, fmt.Errorf("READ_BLOCK must be a positive duration, got %q", get("READ_BLOCK", "")))
	}
	if cfg.ShutdownTimeout, err = time.ParseDuration(get("SHUTDOWN_TIMEOUT", "30s")); err != nil || cfg.ShutdownTimeout < 0 {
		errs = append(errs, fmt.Errorf("SHUTDOWN_TIMEOUT must be a duration, got %q", get("SHUTDOWN_TIMEOUT", "")))
	}
	if err := cfg.LogLevel.UnmarshalText([]byte(get("LOG_LEVEL", "info"))); err != nil {
		errs = append(errs, fmt.Errorf("LOG_LEVEL: %w", err))
	}
	if cfg.LogFormat != "json" && cfg.LogFormat != "text" {
		errs = append(errs, fmt.Errorf("LOG_FORMAT must be json or text, got %q", cfg.LogFormat))
	}

	return cfg, errors.Join(errs...)
}

// defaultWorkerID нужен, чтобы несколько воркеров на одной машине
// не пересекались по имени потребителя в consumer group.
func defaultWorkerID() string {
	host, err := os.Hostname()
	if err != nil {
		host = "worker"
	}
	return fmt.Sprintf("%s-%d", host, os.Getpid())
}
