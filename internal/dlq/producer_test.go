package dlq_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Daniel-Dos/gopayground/internal/dlq"

	"github.com/IBM/sarama"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockSyncProducer struct {
	sendMessageFunc func(msg *sarama.ProducerMessage) (partition int32, offset int64, err error)
}

func (m *mockSyncProducer) SendMessage(msg *sarama.ProducerMessage) (partition int32, offset int64, err error) {
	return m.sendMessageFunc(msg)
}

func (m *mockSyncProducer) SendMessages(msgs []*sarama.ProducerMessage) (map[int32][]int64, error) {
	panic("not implemented")
}

func (m *mockSyncProducer) Close() error {
	return nil
}

func TestDLQPublish_Success(t *testing.T) {
	var capturedMsg *sarama.ProducerMessage

	mockProducer := &mockSyncProducer{
		sendMessageFunc: func(msg *sarama.ProducerMessage) (int32, int64, error) {
			capturedMsg = msg
			return 0, 123, nil
		},
	}

	producer := dlq.NewProducer(mockProducer, "payment.events.dlq")

	originalMsg := &sarama.ConsumerMessage{
		Topic:     "payment.events",
		Partition: 2,
		Offset:    456,
		Key:       []byte("payment-key"),
		Value:     []byte(`{"payment_id":"test"}`),
		Headers: []*sarama.RecordHeader{
			{Key: []byte("trace_id"), Value: []byte("abc123")},
		},
	}

	err := producer.Publish(context.Background(), originalMsg, errors.New("processing failed"))
	require.NoError(t, err)

	require.NotNil(t, capturedMsg)
	assert.Equal(t, "payment.events.dlq", capturedMsg.Topic)
	// Key and Value are sarama.ByteEncoder, convert to compare
	keyBytes, _ := capturedMsg.Key.Encode()
	assert.Equal(t, []byte("payment-key"), keyBytes)
	valBytes, _ := capturedMsg.Value.Encode()
	assert.Equal(t, []byte(`{"payment_id":"test"}`), valBytes)

	// Check headers
	headerMap := make(map[string]string)
	for _, h := range capturedMsg.Headers {
		headerMap[string(h.Key)] = string(h.Value)
	}

	assert.Equal(t, "payment.events", headerMap["original_topic"])
	assert.Equal(t, "2", headerMap["original_partition"])
	assert.Equal(t, "456", headerMap["original_offset"])
	assert.Equal(t, "3", headerMap["error_count"])
	assert.Equal(t, "processing failed", headerMap["last_error"])
	assert.NotEmpty(t, headerMap["dlq_timestamp"])
	// Original headers preserved
	assert.Equal(t, "abc123", headerMap["trace_id"])
}

func TestDLQPublish_PreservesOriginalKey(t *testing.T) {
	var capturedMsg *sarama.ProducerMessage

	mockProducer := &mockSyncProducer{
		sendMessageFunc: func(msg *sarama.ProducerMessage) (int32, int64, error) {
			capturedMsg = msg
			return 0, 0, nil
		},
	}

	producer := dlq.NewProducer(mockProducer, "payment.events.dlq")
	originalMsg := &sarama.ConsumerMessage{
		Topic: "payment.events",
		Key:   []byte("my-custom-key"),
		Value: []byte(`{}`),
	}

	err := producer.Publish(context.Background(), originalMsg, errors.New("error"))
	require.NoError(t, err)
	keyBytes, _ := capturedMsg.Key.Encode()
	assert.Equal(t, []byte("my-custom-key"), keyBytes)
}

func TestDLQPublish_ProducerError(t *testing.T) {
	mockProducer := &mockSyncProducer{
		sendMessageFunc: func(msg *sarama.ProducerMessage) (int32, int64, error) {
			return 0, 0, errors.New("kafka broker not available")
		},
	}

	producer := dlq.NewProducer(mockProducer, "payment.events.dlq")
	originalMsg := &sarama.ConsumerMessage{
		Topic: "payment.events",
		Value: []byte(`{}`),
	}

	err := producer.Publish(context.Background(), originalMsg, errors.New("some error"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "dlq publish error")
}

func TestDLQPublish_HeadersNotMutated(t *testing.T) {
	var capturedMsg *sarama.ProducerMessage

	mockProducer := &mockSyncProducer{
		sendMessageFunc: func(msg *sarama.ProducerMessage) (int32, int64, error) {
			capturedMsg = msg
			return 0, 0, nil
		},
	}

	producer := dlq.NewProducer(mockProducer, "payment.events.dlq")
	originalHeaders := []*sarama.RecordHeader{
		{Key: []byte("custom"), Value: []byte("value")},
	}
	originalMsg := &sarama.ConsumerMessage{
		Topic:   "payment.events",
		Value:   []byte(`{"data":"test"}`),
		Headers: originalHeaders,
	}

	err := producer.Publish(context.Background(), originalMsg, errors.New("err"))
	require.NoError(t, err)

	// Original value unchanged
	valBytes, _ := capturedMsg.Value.Encode()
	assert.Equal(t, []byte(`{"data":"test"}`), valBytes)

	// Both original and new headers present
	headerCount := len(capturedMsg.Headers)
	assert.Equal(t, 7, headerCount) // 1 original + 6 new
}
