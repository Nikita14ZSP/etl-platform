package runner

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/Nikita14ZSP/etl-platform/worker/internal/queue"
	"github.com/Nikita14ZSP/etl-platform/worker/task"
)

// fakeSource отдаёт заранее заданные записи, а потом ведёт себя
// как пустой стрим: ждёт немного и возвращает nil.
type fakeSource struct {
	mu         sync.Mutex
	deliveries []*queue.Delivery
	readErrs   []error
	acked      []string
}

func (s *fakeSource) Read(ctx context.Context) (*queue.Delivery, error) {
	s.mu.Lock()
	if len(s.readErrs) > 0 {
		err := s.readErrs[0]
		s.readErrs = s.readErrs[1:]
		s.mu.Unlock()
		return nil, err
	}
	if len(s.deliveries) > 0 {
		d := s.deliveries[0]
		s.deliveries = s.deliveries[1:]
		s.mu.Unlock()
		return d, nil
	}
	s.mu.Unlock()

	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Millisecond):
	}
	return nil, nil
}

func (s *fakeSource) Ack(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.acked = append(s.acked, id)
	return nil
}

func (s *fakeSource) ackedIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.acked...)
}

func samplePayload(t *testing.T) []byte {
	t.Helper()

	data, err := os.ReadFile("../../task/testdata/task_message_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// runFor запускает Runner и останавливает его, когда cond станет true
// или через секунду.
func runFor(t *testing.T, r *Runner, cond func() bool) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		r.Run(ctx)
		close(done)
	}()

	for !cond() && ctx.Err() == nil {
		time.Sleep(time.Millisecond)
	}
	cancel()
	<-done
}

func TestRunHandlesAndAcksTask(t *testing.T) {
	src := &fakeSource{deliveries: []*queue.Delivery{{ID: "1-0", Payload: samplePayload(t)}}}

	var got []*task.Message
	var mu sync.Mutex
	h := HandlerFunc(func(_ context.Context, msg *task.Message) error {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, msg)
		return nil
	})

	runFor(t, New(src, h, discardLogger()), func() bool { return len(src.ackedIDs()) == 1 })

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 || got[0].TaskID != "transform" {
		t.Fatalf("handler should get the task once, got %v", got)
	}
	if acked := src.ackedIDs(); len(acked) != 1 || acked[0] != "1-0" {
		t.Fatalf("message should be acked, got %v", acked)
	}
}

func TestRunAcksFailedTask(t *testing.T) {
	src := &fakeSource{deliveries: []*queue.Delivery{{ID: "1-0", Payload: samplePayload(t)}}}
	h := HandlerFunc(func(context.Context, *task.Message) error { return errors.New("exit status 1") })

	runFor(t, New(src, h, discardLogger()), func() bool { return len(src.ackedIDs()) == 1 })

	if acked := src.ackedIDs(); len(acked) != 1 {
		t.Fatalf("failed task should still be acked, got %v", acked)
	}
}

func TestRunDropsInvalidMessage(t *testing.T) {
	src := &fakeSource{deliveries: []*queue.Delivery{
		{ID: "1-0", Payload: []byte(`not json`)},
		{ID: "2-0", Payload: nil},
	}}
	called := false
	h := HandlerFunc(func(context.Context, *task.Message) error {
		called = true
		return nil
	})

	runFor(t, New(src, h, discardLogger()), func() bool { return len(src.ackedIDs()) == 2 })

	if called {
		t.Error("handler should not be called for invalid messages")
	}
	if acked := src.ackedIDs(); len(acked) != 2 {
		t.Fatalf("invalid messages should be acked, got %v", acked)
	}
}

func TestRunDoesNotAckInterruptedTask(t *testing.T) {
	src := &fakeSource{deliveries: []*queue.Delivery{{ID: "1-0", Payload: samplePayload(t)}}}

	started := make(chan struct{})
	h := HandlerFunc(func(ctx context.Context, _ *task.Message) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		New(src, h, discardLogger()).Run(ctx)
		close(done)
	}()

	<-started
	cancel()
	<-done

	if acked := src.ackedIDs(); len(acked) != 0 {
		t.Fatalf("interrupted task should not be acked, got %v", acked)
	}
}

func TestRunRetriesAfterReadError(t *testing.T) {
	src := &fakeSource{
		readErrs:   []error{errors.New("connection refused")},
		deliveries: []*queue.Delivery{{ID: "1-0", Payload: samplePayload(t)}},
	}
	r := New(src, HandlerFunc(func(context.Context, *task.Message) error { return nil }), discardLogger())
	r.retryDelay = time.Millisecond

	runFor(t, r, func() bool { return len(src.ackedIDs()) == 1 })

	if acked := src.ackedIDs(); len(acked) != 1 {
		t.Fatalf("runner should recover after read error, got %v", acked)
	}
}
