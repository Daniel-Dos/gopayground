package history

import (
	"context"
	"errors"
	"fmt"

	"github.com/Daniel-Dos/gopayground/internal/models"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.opentelemetry.io/otel/trace"
)

// DynamoDBPutItemAPI defines the interface for the DynamoDB PutItem operation.
type DynamoDBPutItemAPI interface {
	PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
}

// Recorder defines the interface for recording payment history.
type Recorder interface {
	RecordHistory(ctx context.Context, event *models.PaymentEvent) error
}

type dynamoRecorder struct {
	client DynamoDBPutItemAPI
	table  string
}

// NewRecorder creates a new DynamoDB-based history recorder.
func NewRecorder(client DynamoDBPutItemAPI, table string) Recorder {
	return &dynamoRecorder{client: client, table: table}
}

func (dr *dynamoRecorder) RecordHistory(ctx context.Context, event *models.PaymentEvent) error {
	traceID := ""
	if span := trace.SpanFromContext(ctx); span != nil && span.SpanContext().HasTraceID() {
		traceID = span.SpanContext().TraceID().String()
	}

	history := models.NewPaymentHistoryFromEvent(event, traceID)

	item, err := attributevalue.MarshalMap(history)
	if err != nil {
		return fmt.Errorf("marshal error: %w", err)
	}

	_, err = dr.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(dr.table),
		Item:      item,
		ConditionExpression: aws.String("attribute_not_exists(payment_id) AND attribute_not_exists(#ts)"),
		ExpressionAttributeNames: map[string]string{
			"#ts": "timestamp",
		},
	})
	if err != nil {
		var condErr *types.ConditionalCheckFailedException
		if errors.As(err, &condErr) {
			return nil // already recorded, not an error
		}
		return fmt.Errorf("dynamodb put error: %w", err)
	}

	return nil
}
