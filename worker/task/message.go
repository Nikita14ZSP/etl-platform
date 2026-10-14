// Package task описывает сообщение, которое планировщик кладёт в очередь,
// а воркер забирает и исполняет (контракт 1 из концепции проекта).
package task

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// SchemaVersion — версия контракта, которую понимает воркер.
const SchemaVersion = 1

// ErrUnsupportedSchema возвращается, если планировщик прислал сообщение
// версии, о которой воркер ещё не знает.
var ErrUnsupportedSchema = errors.New("unsupported schema version")

// Message — одна попытка выполнения задачи.
type Message struct {
	SchemaVersion  int    `json:"schema_version"`
	TaskInstanceID string `json:"task_instance_id"`
	DagID          string `json:"dag_id"`
	RunID          string `json:"run_id"`
	TaskID         string `json:"task_id"`
	TryNumber      int    `json:"try_number"`
	Operator       string `json:"operator"`

	// Config разбирает сам оператор: у каждого он свой.
	Config json.RawMessage `json:"config"`

	Context    Context   `json:"context"`
	TimeoutSec int       `json:"timeout_sec"`
	QueuedAt   time.Time `json:"queued_at"`
}

// Context — параметры запуска, которые нужны задаче во время выполнения.
type Context struct {
	ExecutionDate string `json:"execution_date"`
	// Результаты upstream-задач по task_id. Значение может быть любым JSON.
	XCom map[string]json.RawMessage `json:"xcom,omitempty"`
}

// Parse разбирает и проверяет сообщение из очереди.
//
// Неизвестные поля не считаются ошибкой: планировщик может добавлять
// новые поля в рамках той же версии, и старые воркеры не должны падать.
func Parse(data []byte) (*Message, error) {
	var m Message
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("decode task message: %w", err)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

// Validate проверяет обязательные поля сообщения.
func (m *Message) Validate() error {
	if m.SchemaVersion != SchemaVersion {
		return fmt.Errorf("%w: %d", ErrUnsupportedSchema, m.SchemaVersion)
	}

	var errs []error
	required := []struct{ name, value string }{
		{"task_instance_id", m.TaskInstanceID},
		{"dag_id", m.DagID},
		{"run_id", m.RunID},
		{"task_id", m.TaskID},
		{"operator", m.Operator},
	}
	for _, f := range required {
		if f.value == "" {
			errs = append(errs, fmt.Errorf("%s is required", f.name))
		}
	}

	if m.TryNumber < 1 {
		errs = append(errs, fmt.Errorf("try_number must be >= 1, got %d", m.TryNumber))
	}
	if m.TimeoutSec <= 0 {
		errs = append(errs, fmt.Errorf("timeout_sec must be > 0, got %d", m.TimeoutSec))
	}
	if !isJSONObject(m.Config) {
		errs = append(errs, errors.New("config must be a JSON object"))
	}
	if _, err := time.Parse(time.DateOnly, m.Context.ExecutionDate); err != nil {
		errs = append(errs, fmt.Errorf("context.execution_date must be YYYY-MM-DD, got %q", m.Context.ExecutionDate))
	}

	if len(errs) > 0 {
		return fmt.Errorf("invalid task message %q: %w", m.TaskInstanceID, errors.Join(errs...))
	}
	return nil
}

// Timeout возвращает лимит времени на выполнение задачи.
func (m *Message) Timeout() time.Duration {
	return time.Duration(m.TimeoutSec) * time.Second
}

func isJSONObject(raw json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	return len(raw) > 0 && raw[0] == '{'
}
