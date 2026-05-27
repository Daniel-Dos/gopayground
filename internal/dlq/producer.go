package dlq

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/IBM/sarama"
	"go.opentelemetry.io/otel/trace"
)

// SyncProducer defines the minimal interface for producing messages.
type SyncProducer interface {
	SendMessage(msg *sarama.ProducerMessage) (partition int32, offset int64, err error)
}

// Producer defines the interface for publishing messages to DLQ.
type Producer interface {
	Publish(ctx context.Context, msg *sarama.ConsumerMessage, err error) error
}

type kafkaDLQProducer struct {
	producer SyncProducer
	topic    string
}

// NewProducer creates a new Kafka DLQ producer.
func NewProducer(producer SyncProducer, topic string) Producer {
	return &kafkaDLQProducer{producer: producer, topic: topic}
}

func (kp *kafkaDLQProducer) Publish(ctx context.Context, msg *sarama.ConsumerMessage, err error) error {
	// Extract trace_id from span context for DLQ header
	traceID := ""
	if span := trace.SpanFromContext(ctx); span != nil && span.SpanContext().HasTraceID() {
		traceID = span.SpanContext().TraceID().String()
	}

	newHeaders := []sarama.RecordHeader{
		{Key: []byte("original_topic"), Value: []byte(msg.Topic)},
		{Key: []byte("original_partition"), Value: []byte(strconv.Itoa(int(msg.Partition)))},
		{Key: []byte("original_offset"), Value: []byte(strconv.FormatInt(msg.Offset, 10))},
		{Key: []byte("error_count"), Value: []byte("3")},
		{Key: []byte("last_error"), Value: []byte(err.Error())},
		{Key: []byte("dlq_timestamp"), Value: []byte(time.Now().UTC().Format(time.RFC3339))},
	}
	if traceID != "" {
		newHeaders = append(newHeaders, sarama.RecordHeader{
			Key: []byte("trace_id"), Value: []byte(traceID),
		})
	}

	allHeaders := make([]sarama.RecordHeader, 0, len(msg.Headers)+len(newHeaders))
	for _, h := range msg.Headers {
		if h != nil {
			allHeaders = append(allHeaders, *h)
		}
	}
	allHeaders = append(allHeaders, newHeaders...)

	dlqMsg := &sarama.ProducerMessage{
		Topic:   kp.topic,
		Key:     sarama.ByteEncoder(msg.Key),
		Value:   sarama.ByteEncoder(msg.Value),
		Headers: allHeaders,
	}

	// Enforce context timeout for SendMessage (sarama's SendMessage doesn't accept context)
	doneCh := make(chan error, 1)
	go func() {
		_, _, sendErr := kp.producer.SendMessage(dlqMsg)
		doneCh <- sendErr
	}()

	select {
	case <-ctx.Done():
		return fmt.Errorf("dlq publish cancelled: %w", ctx.Err())
	case sendErr := <-doneCh:
		if sendErr != nil {
			return fmt.Errorf("dlq publish error: %w", sendErr)
		}
		return nil
	}
}
