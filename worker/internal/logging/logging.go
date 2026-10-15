// Package logging настраивает slog для воркера.
package logging

import (
	"io"
	"log/slog"
)

// New возвращает логгер, в каждую запись которого добавлен worker_id,
// чтобы логи нескольких воркеров можно было различить.
func New(w io.Writer, format string, level slog.Level, workerID string) *slog.Logger {
	opts := &slog.HandlerOptions{Level: level}

	var h slog.Handler
	if format == "text" {
		h = slog.NewTextHandler(w, opts)
	} else {
		h = slog.NewJSONHandler(w, opts)
	}

	return slog.New(h).With("worker_id", workerID)
}
