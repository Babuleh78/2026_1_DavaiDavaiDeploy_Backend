package kafka

import (
	"context"
	"time"

	kafkago "github.com/segmentio/kafka-go"
)

const publishTimeout = 5 * time.Second

type KafkaProducer struct {
	brokers []string
}

func NewKafkaProducer(brokers []string) *KafkaProducer {
	return &KafkaProducer{brokers: brokers}
}

func (p *KafkaProducer) writer(topic string) *kafkago.Writer {
	return &kafkago.Writer{
		Addr:         kafkago.TCP(p.brokers...),
		Topic:        topic,
		Balancer:     &kafkago.LeastBytes{},
		RequiredAcks: kafkago.RequireOne,
	}
}

func (p *KafkaProducer) Publish(ctx context.Context, topic, key string, value []byte) error {
	w := p.writer(topic)
	defer w.Close()

	pubCtx, cancel := context.WithTimeout(ctx, publishTimeout)
	defer cancel()

	return w.WriteMessages(pubCtx, kafkago.Message{
		Key:   []byte(key),
		Value: value,
	})
}

func (p *KafkaProducer) PublishAsync(ctx context.Context, topic, key string, value []byte, errFn func(error)) {
	go func() {
		if err := p.Publish(context.Background(), topic, key, value); err != nil && errFn != nil {
			errFn(err)
		}
	}()
}
