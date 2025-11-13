package kafka

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/segmentio/kafka-go"
)

// Create a Kafka writer (producer)
func NewWriter(broker, topic string) *kafka.Writer {
	return kafka.NewWriter(kafka.WriterConfig{
		Brokers:  []string{broker},
		Topic:    topic,
		Balancer: &kafka.LeastBytes{},
	})
}

// Publish a single symbol-price pair
func PublishPrice(ctx context.Context, w *kafka.Writer, symbol string, price float64) {
	msg := kafka.Message{
		Key:   []byte(symbol),
		Value: []byte(fmt.Sprintf("%.8f", price)),
		Time:  time.Now(),
	}
	if err := w.WriteMessages(ctx, msg); err != nil {
		log.Printf("⚠️ [KAFKA] Failed to publish %s: %v\n", symbol, err)
	} else {
		log.Printf("📤 [KAFKA] Published %s => %.8f\n", symbol, price)
	}
}

// 🚀 Publish multiple Redis pairs to Kafka
func PublishAllFromRedis(ctx context.Context, w *kafka.Writer, data map[string]string) {
	for k, v := range data {
		msg := kafka.Message{
			Key:   []byte(k),
			Value: []byte(v),
			Time:  time.Now(),
		}
		if err := w.WriteMessages(ctx, msg); err != nil {
			log.Printf("⚠️ [KAFKA] Failed to publish %s: %v\n", k, err)
		}
	}
	log.Printf("✅ [KAFKA] Published %d Redis pairs to Kafka topic\n", len(data))
}
