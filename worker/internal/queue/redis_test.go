package queue

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// Интеграционные тесты: нужен запущенный Redis.
// Адрес берётся из TEST_REDIS_URL, по умолчанию локальный Redis, база 15.
// Если Redis недоступен, тесты пропускаются.
func newTestClient(t *testing.T) *redis.Client {
	t.Helper()

	url := os.Getenv("TEST_REDIS_URL")
	if url == "" {
		url = "redis://localhost:6379/15"
	}
	opts, err := redis.ParseURL(url)
	if err != nil {
		t.Fatalf("bad TEST_REDIS_URL: %v", err)
	}

	rdb := redis.NewClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		_ = rdb.Close()
		t.Skipf("redis is not available at %s: %v", url, err)
	}

	t.Cleanup(func() { _ = rdb.Close() })
	return rdb
}

func testStream(t *testing.T, rdb *redis.Client) string {
	t.Helper()

	stream := fmt.Sprintf("test:%s:%d", t.Name(), time.Now().UnixNano())
	t.Cleanup(func() { rdb.Del(context.Background(), stream) })
	return stream
}

func TestEnsureGroupIsIdempotent(t *testing.T) {
	rdb := newTestClient(t)
	stream := testStream(t, rdb)
	c := NewConsumer(rdb, stream, "workers", "w1", 100*time.Millisecond)

	ctx := context.Background()
	if err := c.EnsureGroup(ctx); err != nil {
		t.Fatalf("first EnsureGroup: %v", err)
	}
	if err := c.EnsureGroup(ctx); err != nil {
		t.Fatalf("second EnsureGroup should not fail: %v", err)
	}
}

func TestReadAndAck(t *testing.T) {
	rdb := newTestClient(t)
	stream := testStream(t, rdb)
	ctx := context.Background()

	c := NewConsumer(rdb, stream, "workers", "w1", 100*time.Millisecond)
	if err := c.EnsureGroup(ctx); err != nil {
		t.Fatal(err)
	}

	id, err := Publish(ctx, rdb, stream, []byte(`{"task_id":"t1"}`))
	if err != nil {
		t.Fatal(err)
	}

	d, err := c.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if d == nil || d.ID != id || string(d.Payload) != `{"task_id":"t1"}` {
		t.Fatalf("unexpected delivery: %+v", d)
	}

	// До подтверждения запись висит в pending у этого потребителя.
	pending, err := rdb.XPending(ctx, stream, "workers").Result()
	if err != nil {
		t.Fatal(err)
	}
	if pending.Count != 1 || pending.Consumers["w1"] != 1 {
		t.Fatalf("expected 1 pending message for w1, got %+v", pending)
	}

	if err := c.Ack(ctx, d.ID); err != nil {
		t.Fatal(err)
	}
	pending, err = rdb.XPending(ctx, stream, "workers").Result()
	if err != nil {
		t.Fatal(err)
	}
	if pending.Count != 0 {
		t.Fatalf("expected no pending messages after ack, got %d", pending.Count)
	}
}

func TestMessageGoesToOneConsumer(t *testing.T) {
	rdb := newTestClient(t)
	stream := testStream(t, rdb)
	ctx := context.Background()

	c1 := NewConsumer(rdb, stream, "workers", "w1", 100*time.Millisecond)
	c2 := NewConsumer(rdb, stream, "workers", "w2", 100*time.Millisecond)
	if err := c1.EnsureGroup(ctx); err != nil {
		t.Fatal(err)
	}

	if _, err := Publish(ctx, rdb, stream, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}

	d1, err := c1.Read(ctx)
	if err != nil || d1 == nil {
		t.Fatalf("first consumer should get the message: %v, %v", d1, err)
	}
	d2, err := c2.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if d2 != nil {
		t.Fatalf("second consumer should get nothing, got %+v", d2)
	}
}

func TestReadTimeout(t *testing.T) {
	rdb := newTestClient(t)
	stream := testStream(t, rdb)
	ctx := context.Background()

	c := NewConsumer(rdb, stream, "workers", "w1", 100*time.Millisecond)
	if err := c.EnsureGroup(ctx); err != nil {
		t.Fatal(err)
	}

	d, err := c.Read(ctx)
	if err != nil || d != nil {
		t.Fatalf("expected nil, nil on empty stream, got %+v, %v", d, err)
	}
}

func TestReadEntryWithoutPayload(t *testing.T) {
	rdb := newTestClient(t)
	stream := testStream(t, rdb)
	ctx := context.Background()

	c := NewConsumer(rdb, stream, "workers", "w1", 100*time.Millisecond)
	if err := c.EnsureGroup(ctx); err != nil {
		t.Fatal(err)
	}
	if err := rdb.XAdd(ctx, &redis.XAddArgs{Stream: stream, Values: map[string]any{"foo": "bar"}}).Err(); err != nil {
		t.Fatal(err)
	}

	d, err := c.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if d == nil || len(d.Payload) != 0 {
		t.Fatalf("expected delivery with empty payload, got %+v", d)
	}
}
