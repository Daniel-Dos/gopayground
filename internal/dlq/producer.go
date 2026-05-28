package dlq

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/Daniel-Dos/gopayground/internal/kafka"

	"github.com/IBM/sarama"
	"go.opentelemetry.io/otel/trace"
)

// Producer define a interface para publicar mensagens na DLQ (Dead Letter Queue).
type Producer interface {
	Publish(ctx context.Context, msg *sarama.ConsumerMessage, err error) error
}

type kafkaDLQProducer struct {
	producer kafka.SyncProducer
	topic    string
}

// NewProducer cria um novo produtor Kafka para a DLQ.
func NewProducer(producer kafka.SyncProducer, topic string) Producer {
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

	return kp.sendWithContext(ctx, dlqMsg)
}

// sendWithContext encapsula o SendMessage do Sarama com suporte a contexto.
func (kp *kafkaDLQProducer) sendWithContext(ctx context.Context, msg *sarama.ProducerMessage) error {
	type result struct {
		err error
	}

	ch := make(chan result, 1)
	go func() {
		_, _, sendErr := kp.producer.SendMessage(msg)
		ch <- result{err: sendErr}
	}()

	select {
	case <-ctx.Done():
		return fmt.Errorf("dlq publish cancelled: %w", ctx.Err())
	case r := <-ch:
		if r.err != nil {
			return fmt.Errorf("dlq publish error: %w", r.err)
		}
		return nil
	}
}
