// Package queue работает с очередью задач в Redis Streams.
//
// Сообщение лежит в записи стрима в поле payload в виде JSON (см. пакет task).
// Каждое сообщение достаётся ровно одному потребителю из consumer group
// и висит в pending, пока его не подтвердят через XACK.
package queue

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// PayloadField — поле записи стрима, в котором лежит сообщение.
const PayloadField = "payload"

// Delivery — запись, полученная из стрима.
type Delivery struct {
	ID      string
	Payload []byte
}

// Consumer читает задачи из стрима от имени одного потребителя группы.
type Consumer struct {
	rdb    *redis.Client
	stream string
	group  string
	name   string
	block  time.Duration
}

// NewConsumer создаёт потребителя. name должен быть уникальным
// для каждого воркера, обычно это WORKER_ID.
func NewConsumer(rdb *redis.Client, stream, group, name string, block time.Duration) *Consumer {
	return &Consumer{rdb: rdb, stream: stream, group: group, name: name, block: block}
}

// EnsureGroup создаёт consumer group, если её ещё нет.
// Группа начинает с начала стрима, чтобы не потерять задачи,
// которые планировщик успел положить до первого запуска воркеров.
func (c *Consumer) EnsureGroup(ctx context.Context) error {
	err := c.rdb.XGroupCreateMkStream(ctx, c.stream, c.group, "0").Err()
	if err != nil && !strings.HasPrefix(err.Error(), "BUSYGROUP") {
		return fmt.Errorf("create consumer group %s: %w", c.group, err)
	}
	return nil
}

// Read ждёт следующую новую задачу не дольше block.
// Если за это время ничего не пришло, возвращает nil, nil.
func (c *Consumer) Read(ctx context.Context) (*Delivery, error) {
	res, err := c.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    c.group,
		Consumer: c.name,
		Streams:  []string{c.stream, ">"},
		Count:    1,
		Block:    c.block,
	}).Result()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read from stream %s: %w", c.stream, err)
	}
	if len(res) == 0 || len(res[0].Messages) == 0 {
		return nil, nil
	}

	msg := res[0].Messages[0]
	d := &Delivery{ID: msg.ID}
	// Запись без payload всё равно возвращаем: её нужно подтвердить,
	// иначе она навсегда останется в pending.
	if v, ok := msg.Values[PayloadField].(string); ok {
		d.Payload = []byte(v)
	}
	return d, nil
}

// Ack подтверждает обработку записи.
func (c *Consumer) Ack(ctx context.Context, id string) error {
	if err := c.rdb.XAck(ctx, c.stream, c.group, id).Err(); err != nil {
		return fmt.Errorf("ack %s: %w", id, err)
	}
	return nil
}

// Publish кладёт сообщение в стрим и возвращает ID записи.
// В проде задачи публикует планировщик, здесь это нужно для утилит и тестов.
func Publish(ctx context.Context, rdb *redis.Client, stream string, payload []byte) (string, error) {
	id, err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		Values: map[string]any{PayloadField: payload},
	}).Result()
	if err != nil {
		return "", fmt.Errorf("publish to stream %s: %w", stream, err)
	}
	return id, nil
}
