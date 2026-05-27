package history

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// DynamoDBTableAPI defines the interface for DynamoDB table operations.
// It is satisfied by *dynamodb.Client and is compatible with the
// DynamoDB waiter API for structural typing.
type DynamoDBTableAPI interface {
	DescribeTable(ctx context.Context, params *dynamodb.DescribeTableInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DescribeTableOutput, error)
	CreateTable(ctx context.Context, params *dynamodb.CreateTableInput, optFns ...func(*dynamodb.Options)) (*dynamodb.CreateTableOutput, error)
}

// EnsureTable checks if the DynamoDB table exists and creates it if it
// doesn't. It uses PAY_PER_REQUEST billing mode (on-demand) and waits
// for the table to become ACTIVE before returning (up to 30s timeout).
//
// The table schema matches the PaymentHistory model:
//   - Partition key: payment_id (string)
//   - Sort key:      timestamp  (string)
func EnsureTable(ctx context.Context, client DynamoDBTableAPI, tableName string, logger *slog.Logger) error {
	// 1. Check if table already exists
	_, err := client.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(tableName),
	})
	if err == nil {
		logger.Info("dynamodb table already exists", "table", tableName)
		return nil
	}

	var notFound *types.ResourceNotFoundException
	if !errors.As(err, &notFound) {
		return fmt.Errorf("describe table %s: %w", tableName, err)
	}

	// 2. Table does not exist — create it
	logger.Info("creating dynamodb table",
		"table", tableName,
		"partition_key", "payment_id",
		"sort_key", "timestamp",
		"billing_mode", "PAY_PER_REQUEST",
	)

	_, err = client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(tableName),
		AttributeDefinitions: []types.AttributeDefinition{
			{
				AttributeName: aws.String("payment_id"),
				AttributeType: types.ScalarAttributeTypeS,
			},
			{
				AttributeName: aws.String("timestamp"),
				AttributeType: types.ScalarAttributeTypeS,
			},
		},
		KeySchema: []types.KeySchemaElement{
			{
				AttributeName: aws.String("payment_id"),
				KeyType:       types.KeyTypeHash,
			},
			{
				AttributeName: aws.String("timestamp"),
				KeyType:       types.KeyTypeRange,
			},
		},
		BillingMode: types.BillingModePayPerRequest,
	})
	if err != nil {
		return fmt.Errorf("create table %s: %w", tableName, err)
	}

	// 3. Wait for the table to become ACTIVE (up to 30s)
	logger.Info("waiting for dynamodb table to become active", "table", tableName)
	waiter := dynamodb.NewTableExistsWaiter(client)
	waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if err := waiter.Wait(waitCtx, &dynamodb.DescribeTableInput{
		TableName: aws.String(tableName),
	}, 30*time.Second); err != nil {
		return fmt.Errorf("wait for table %s to become active: %w", tableName, err)
	}

	logger.Info("dynamodb table is active and ready", "table", tableName)
	return nil
}
