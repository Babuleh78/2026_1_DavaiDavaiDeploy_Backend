package kafka

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"time"

	kafkago "github.com/segmentio/kafka-go"
)

type MessageHandler func(ctx context.Context, topic, key string, value []byte) error

type KafkaConsumer struct {
	brokers []string
	logger  *slog.Logger
}

func NewKafkaConsumer(brokers []string, logger *slog.Logger) *KafkaConsumer {
	return &KafkaConsumer{brokers: brokers, logger: logger}
}

func (c *KafkaConsumer) Subscribe(ctx context.Context, topic, groupID string, handler MessageHandler) error {
	const (
		initialDelay = time.Second
		maxDelay     = 60 * time.Second
	)
	delay := initialDelay
	attempt := 0

	for {
		err := c.subscribe(ctx, topic, groupID, handler)
		if err == nil {
			return nil
		}

		attempt++
		c.logger.Warn("kafka consumer error — will retry",
			"topic", topic,
			"group", groupID,
			"attempt", attempt,
			"retry_in", delay.String(),
			"error", err,
		)

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(delay):
		}

		delay *= 2
		if delay > maxDelay {
			delay = maxDelay
		}
	}
}

func (c *KafkaConsumer) subscribe(ctx context.Context, topic, groupID string, handler MessageHandler) error {
	r := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:  c.brokers,
		Topic:    topic,
		GroupID:  groupID,
		MinBytes: 1,
		MaxBytes: 10e6,
	})
	defer r.Close()

	for {
		msg, err := r.FetchMessage(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, io.EOF) {
				return nil
			}
			c.logger.Warn("kafka fetch error", "topic", topic, "group", groupID, "error", err)
			return err
		}

		if handlerErr := handler(ctx, msg.Topic, string(msg.Key), msg.Value); handlerErr != nil {
			c.logger.Warn("kafka message handler error — skipping commit",
				"topic", msg.Topic,
				"partition", msg.Partition,
				"offset", msg.Offset,
				"error", handlerErr,
			)
			continue
		}

		if commitErr := r.CommitMessages(ctx, msg); commitErr != nil {
			c.logger.Warn("kafka commit error", "topic", topic, "error", commitErr)
		}
	}
}
