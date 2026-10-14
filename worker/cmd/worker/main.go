// Команда worker забирает задачи из очереди и исполняет их.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Nikita14ZSP/etl-platform/worker/internal/config"
	"github.com/Nikita14ZSP/etl-platform/worker/internal/logging"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error:\n%v\n", err)
		os.Exit(1)
	}

	log := logging.New(os.Stdout, cfg.LogFormat, cfg.LogLevel, cfg.WorkerID)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Info("worker started", "stream", cfg.Stream, "group", cfg.ConsumerGroup)
	<-ctx.Done()
	log.Info("worker stopped")
}
