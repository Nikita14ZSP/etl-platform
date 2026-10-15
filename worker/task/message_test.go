package task

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func loadSample(t *testing.T) map[string]any {
	t.Helper()

	data, err := os.ReadFile("testdata/task_message_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func encode(t *testing.T, m map[string]any) []byte {
	t.Helper()

	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestParseSample(t *testing.T) {
	data, err := os.ReadFile("testdata/task_message_v1.json")
	if err != nil {
		t.Fatal(err)
	}

	msg, err := Parse(data)
	if err != nil {
		t.Fatalf("sample message should be valid: %v", err)
	}

	if msg.TaskInstanceID != "ti_01H8XGJWBWBAQ4Z4G5V9Q1SRKN" || msg.TaskID != "transform" || msg.Operator != "python" {
		t.Errorf("unexpected fields: %+v", msg)
	}
	if msg.TryNumber != 2 {
		t.Errorf("TryNumber = %d, want 2", msg.TryNumber)
	}
	if msg.Timeout() != 30*time.Minute {
		t.Errorf("Timeout() = %v, want 30m", msg.Timeout())
	}
	if want := time.Date(2026, 9, 16, 3, 0, 4, 0, time.UTC); !msg.QueuedAt.Equal(want) {
		t.Errorf("QueuedAt = %v, want %v", msg.QueuedAt, want)
	}

	var cfg struct {
		Script string `json:"script"`
	}
	if err := json.Unmarshal(msg.Config, &cfg); err != nil || cfg.Script != "transforms/join_sales.py" {
		t.Errorf("config was not kept as is: %s", msg.Config)
	}
	if _, ok := msg.Context.XCom["extract_orders"]; !ok {
		t.Error("xcom from upstream task is missing")
	}
}

func TestParseIgnoresUnknownFields(t *testing.T) {
	m := loadSample(t)
	m["priority"] = 5

	if _, err := Parse(encode(t, m)); err != nil {
		t.Fatalf("unknown fields should be ignored: %v", err)
	}
}

func TestParseUnsupportedSchema(t *testing.T) {
	m := loadSample(t)
	m["schema_version"] = 2

	_, err := Parse(encode(t, m))
	if !errors.Is(err, ErrUnsupportedSchema) {
		t.Fatalf("expected ErrUnsupportedSchema, got %v", err)
	}
}

func TestParseInvalid(t *testing.T) {
	tests := []struct {
		name    string
		change  func(m map[string]any)
		wantErr string
	}{
		{"no task instance id", func(m map[string]any) { delete(m, "task_instance_id") }, "task_instance_id"},
		{"empty operator", func(m map[string]any) { m["operator"] = "" }, "operator"},
		{"zero try number", func(m map[string]any) { m["try_number"] = 0 }, "try_number"},
		{"no timeout", func(m map[string]any) { delete(m, "timeout_sec") }, "timeout_sec"},
		{"config is not an object", func(m map[string]any) { m["config"] = "echo hello" }, "config"},
		{"no config", func(m map[string]any) { delete(m, "config") }, "config"},
		{"bad execution date", func(m map[string]any) {
			m["context"] = map[string]any{"execution_date": "16.09.2026"}
		}, "execution_date"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := loadSample(t)
			tt.change(m)

			_, err := Parse(encode(t, m))
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q should mention %s", err, tt.wantErr)
			}
		})
	}
}

func TestParseBrokenJSON(t *testing.T) {
	if _, err := Parse([]byte(`{"schema_version": 1,`)); err == nil {
		t.Fatal("expected an error for broken JSON")
	}
}
