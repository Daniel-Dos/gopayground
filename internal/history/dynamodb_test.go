package history_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Daniel-Dos/gopayground/internal/history"
	"github.com/Daniel-Dos/gopayground/internal/models"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockDynamoClient struct {
	putItemFunc func(ctx context.Context, input *dynamodb.PutItemInput, opts ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
}

func (m *mockDynamoClient) PutItem(ctx context.Context, input *dynamodb.PutItemInput, opts ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	return m.putItemFunc(ctx, input, opts...)
}

func TestRecordHistory_Success(t *testing.T) {
	client := &mockDynamoClient{
		putItemFunc: func(ctx context.Context, input *dynamodb.PutItemInput, opts ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
			assert.Equal(t, "payment_history", *input.TableName)
			assert.NotNil(t, input.ConditionExpression)
			return &dynamodb.PutItemOutput{}, nil
		},
	}

	recorder := history.NewRecorder(client, "payment_history")
	event := &models.PaymentEvent{
		PaymentID:   "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		Status:      "confirmed",
		Amount:      100.0,
		Currency:    "USD",
		Description: "test",
		Timestamp:   "2026-05-24T10:00:00Z",
	}

	err := recorder.RecordHistory(context.Background(), event)
	require.NoError(t, err)
}

func TestRecordHistory_Duplicate(t *testing.T) {
	client := &mockDynamoClient{
		putItemFunc: func(ctx context.Context, input *dynamodb.PutItemInput, opts ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
			return nil, &types.ConditionalCheckFailedException{
				Message: aws.String("condition failed"),
			}
		},
	}

	recorder := history.NewRecorder(client, "payment_history")
	event := &models.PaymentEvent{
		PaymentID:   "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		Status:      "confirmed",
		Amount:      100.0,
		Currency:    "USD",
		Timestamp:   "2026-05-24T10:00:00Z",
	}

	err := recorder.RecordHistory(context.Background(), event)
	require.NoError(t, err) // duplicate should not be an error
}

func TestRecordHistory_DynamoDBError(t *testing.T) {
	client := &mockDynamoClient{
		putItemFunc: func(ctx context.Context, input *dynamodb.PutItemInput, opts ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
			return nil, errors.New("dynamodb is down")
		},
	}

	recorder := history.NewRecorder(client, "payment_history")
	event := &models.PaymentEvent{
		PaymentID:   "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		Status:      "confirmed",
		Amount:      100.0,
		Currency:    "USD",
		Timestamp:   "2026-05-24T10:00:00Z",
	}

	err := recorder.RecordHistory(context.Background(), event)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "dynamodb put error")
}

func TestRecordHistory_ContextCancelled(t *testing.T) {
	client := &mockDynamoClient{
		putItemFunc: func(ctx context.Context, input *dynamodb.PutItemInput, opts ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
			// Simulate context cancellation
			_ = ctx.Err()
			return nil, errors.New("context canceled")
		},
	}

	recorder := history.NewRecorder(client, "payment_history")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	event := &models.PaymentEvent{
		PaymentID:   "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		Status:      "confirmed",
		Amount:      100.0,
		Currency:    "USD",
		Timestamp:   "2026-05-24T10:00:00Z",
	}

	err := recorder.RecordHistory(ctx, event)
	assert.Error(t, err)
}

func TestRecordHistory_AllFields(t *testing.T) {
	client := &mockDynamoClient{
		putItemFunc: func(ctx context.Context, input *dynamodb.PutItemInput, opts ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
			assert.Equal(t, "payment_history", *input.TableName)
			// Verify all expected attributes are present
			expectedKeys := []string{"payment_id", "status", "amount", "currency", "description", "timestamp", "processed_at", "trace_id"}
			for _, k := range expectedKeys {
				_, ok := input.Item[k]
				assert.True(t, ok, "missing attribute: %s", k)
			}
			return &dynamodb.PutItemOutput{}, nil
		},
	}

	recorder := history.NewRecorder(client, "payment_history")
	event := &models.PaymentEvent{
		PaymentID:   "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		Status:      "refunded",
		Amount:      50.0,
		Currency:    "EUR",
		Description: "refund",
		Timestamp:   "2026-05-24T10:00:00Z",
	}

	err := recorder.RecordHistory(context.Background(), event)
	require.NoError(t, err)
}
