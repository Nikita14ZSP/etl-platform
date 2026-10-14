// Package runner содержит основной цикл воркера:
// взять сообщение из очереди → выполнить задачу → подтвердить сообщение.
package runner

import (
	"context"
	"log/slog"
	"time"

	"github.com/Nikita14ZSP/etl-platform/worker/internal/queue"
	"github.com/Nikita14ZSP/etl-platform/worker/task"
)

// Source — откуда берутся задачи. В проде это queue.Consumer.
type Source interface {
	Read(ctx context.Context) (*queue.Delivery, error)
	Ack(ctx context.Context, id string) error
}

// Handler выполняет одну задачу. Ошибка означает, что задача упала.
type Handler interface {
	Handle(ctx context.Context, msg *task.Message) error
}

// HandlerFunc позволяет использовать обычную функцию как Handler.
type HandlerFunc func(ctx context.Context, msg *task.Message) error

// Handle вызывает f(ctx, msg).
func (f HandlerFunc) Handle(ctx context.Context, msg *task.Message) error {
	return f(ctx, msg)
}

// Runner обрабатывает задачи по одной, пока не отменён контекст.
type Runner struct {
	src     Source
	handler Handler
	log     *slog.Logger

	// Пауза перед повторным чтением, если Redis недоступен.
	retryDelay time.Duration
}

// New создаёт Runner.
func New(src Source, handler Handler, log *slog.Logger) *Runner {
	return &Runner{src: src, handler: handler, log: log, retryDelay: time.Second}
}

// Run крутит цикл до отмены ctx.
func (r *Runner) Run(ctx context.Context) {
	for ctx.Err() == nil {
		d, err := r.src.Read(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			r.log.Error("failed to read task", "err", err)
			sleep(ctx, r.retryDelay)
			continue
		}
		if d == nil {
			continue // за время ожидания ничего не пришло
		}
		r.process(ctx, d)
	}
}

func (r *Runner) process(ctx context.Context, d *queue.Delivery) {
	log := r.log.With("message_id", d.ID)

	msg, err := task.Parse(d.Payload)
	if err != nil {
		// Битое сообщение не станет валидным при повторной доставке,
		// поэтому подтверждаем его сразу, иначе оно зависнет в pending.
		log.Error("dropping invalid task message", "err", err)
		r.ack(ctx, log, d.ID)
		return
	}

	log = log.With(
		"task_instance_id", msg.TaskInstanceID,
		"dag_id", msg.DagID,
		"task_id", msg.TaskID,
		"try_number", msg.TryNumber,
	)
	log.Info("task started", "operator", msg.Operator)

	start := time.Now()
	err = r.handler.Handle(ctx, msg)

	if ctx.Err() != nil {
		// Воркер останавливается посреди задачи. Не подтверждаем:
		// сообщение останется в pending и не потеряется.
		log.Warn("task interrupted by shutdown")
		return
	}
	if err != nil {
		log.Error("task failed", "err", err, "duration", time.Since(start))
	} else {
		log.Info("task finished", "duration", time.Since(start))
	}

	// Подтверждаем и упавшую задачу: повторы решает планировщик,
	// он пришлёт новое сообщение со следующим try_number.
	r.ack(ctx, log, d.ID)
}

func (r *Runner) ack(ctx context.Context, log *slog.Logger, id string) {
	if err := r.src.Ack(ctx, id); err != nil {
		log.Error("failed to ack message", "err", err)
	}
}

func sleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}
