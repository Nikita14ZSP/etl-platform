// Команда worker забирает задачи из очереди и исполняет их.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Println("worker started")
	<-ctx.Done()
	log.Println("worker stopped")
}
