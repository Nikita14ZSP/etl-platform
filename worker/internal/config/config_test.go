package config

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		v, ok := m[key]
		return v, ok
	}
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := load(env(map[string]string{"REDIS_URL": "redis://localhost:6379/0"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.WorkerID == "" {
		t.Error("WorkerID should have a default value")
	}
	if cfg.Stream != "etl:tasks" || cfg.ConsumerGroup != "etl-workers" {
		t.Errorf("unexpected stream/group: %q/%q", cfg.Stream, cfg.ConsumerGroup)
	}
	if cfg.Concurrency != 1 {
		t.Errorf("Concurrency = %d, want 1", cfg.Concurrency)
	}
	if cfg.ReadBlock != 5*time.Second || cfg.ShutdownTimeout != 30*time.Second {
		t.Errorf("unexpected durations: %v, %v", cfg.ReadBlock, cfg.ShutdownTimeout)
	}
	if cfg.LogLevel != slog.LevelInfo || cfg.LogFormat != "json" {
		t.Errorf("unexpected log settings: %v, %q", cfg.LogLevel, cfg.LogFormat)
	}
}

func TestLoadOverrides(t *testing.T) {
	cfg, err := load(env(map[string]string{
		"REDIS_URL":        "redis://redis:6379/1",
		"WORKER_ID":        "worker-1",
		"TASK_STREAM":      "tasks",
		"CONSUMER_GROUP":   "group",
		"CONCURRENCY":      "4",
		"READ_BLOCK":       "1s",
		"SHUTDOWN_TIMEOUT": "10s",
		"LOG_LEVEL":        "debug",
		"LOG_FORMAT":       "text",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.WorkerID != "worker-1" || cfg.Stream != "tasks" || cfg.ConsumerGroup != "group" {
		t.Errorf("overrides not applied: %+v", cfg)
	}
	if cfg.Concurrency != 4 || cfg.ReadBlock != time.Second || cfg.ShutdownTimeout != 10*time.Second {
		t.Errorf("overrides not applied: %+v", cfg)
	}
	if cfg.LogLevel != slog.LevelDebug || cfg.LogFormat != "text" {
		t.Errorf("overrides not applied: %+v", cfg)
	}
}

func TestLoadErrors(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantErr string
	}{
		{"no redis url", map[string]string{}, "REDIS_URL"},
		{"blank redis url", map[string]string{"REDIS_URL": "  "}, "REDIS_URL"},
		{"zero concurrency", map[string]string{"REDIS_URL": "redis://x", "CONCURRENCY": "0"}, "CONCURRENCY"},
		{"bad concurrency", map[string]string{"REDIS_URL": "redis://x", "CONCURRENCY": "many"}, "CONCURRENCY"},
		{"bad read block", map[string]string{"REDIS_URL": "redis://x", "READ_BLOCK": "5"}, "READ_BLOCK"},
		{"bad log level", map[string]string{"REDIS_URL": "redis://x", "LOG_LEVEL": "loud"}, "LOG_LEVEL"},
		{"bad log format", map[string]string{"REDIS_URL": "redis://x", "LOG_FORMAT": "xml"}, "LOG_FORMAT"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := load(env(tt.env))
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q should mention %s", err, tt.wantErr)
			}
		})
	}
}
