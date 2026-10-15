// Команда enqueue кладёт тестовое задание в очередь,
// чтобы проверить воркер без планировщика.
//
//	enqueue -cmd "echo hello"
//	enqueue -file task.json
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/Nikita14ZSP/etl-platform/worker/internal/queue"
	"github.com/Nikita14ZSP/etl-platform/worker/task"
)

func main() {
	var (
		redisURL = flag.String("redis", envOr("REDIS_URL", "redis://localhost:6379/0"), "Redis URL")
		stream   = flag.String("stream", envOr("TASK_STREAM", "etl:tasks"), "stream name")
		command  = flag.String("cmd", "echo hello", "bash command for a generated task")
		file     = flag.String("file", "", "send a task message from JSON file instead of generating one")
		timeout  = flag.Int("timeout", 60, "timeout_sec for a generated task")
	)
	flag.Parse()

	if err := run(*redisURL, *stream, *command, *file, *timeout); err != nil {
		fmt.Fprintln(os.Stderr, "enqueue:", err)
		os.Exit(1)
	}
}

func run(redisURL, stream, command, file string, timeout int) error {
	payload, err := buildPayload(command, file, timeout)
	if err != nil {
		return err
	}

	// Проверяем сообщение тем же кодом, что и воркер,
	// чтобы не отправлять в очередь заведомо битое задание.
	msg, err := task.Parse(payload)
	if err != nil {
		return err
	}

	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return fmt.Errorf("parse redis url: %w", err)
	}
	rdb := redis.NewClient(opts)
	defer func() { _ = rdb.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	id, err := queue.Publish(ctx, rdb, stream, payload)
	if err != nil {
		return err
	}

	fmt.Printf("queued %s (%s) as %s\n", msg.TaskInstanceID, msg.Operator, id)
	return nil
}

func buildPayload(command, file string, timeout int) ([]byte, error) {
	if file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("read task file: %w", err)
		}
		return data, nil
	}

	now := time.Now().UTC()
	cfg, err := json.Marshal(map[string]string{"command": command})
	if err != nil {
		return nil, err
	}

	return json.Marshal(task.Message{
		SchemaVersion:  task.SchemaVersion,
		TaskInstanceID: fmt.Sprintf("ti_manual_%d", now.UnixNano()),
		DagID:          "manual",
		RunID:          "manual_" + now.Format(time.RFC3339),
		TaskID:         "bash",
		TryNumber:      1,
		Operator:       "bash",
		Config:         cfg,
		Context:        task.Context{ExecutionDate: now.Format(time.DateOnly)},
		TimeoutSec:     timeout,
		QueuedAt:       now,
	})
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
