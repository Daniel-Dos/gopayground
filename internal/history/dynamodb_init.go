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

// DynamoDBTableAPI define a interface para operações de tabela no DynamoDB.
// É satisfeita por *dynamodb.Client e compatível com a API de waiter do DynamoDB.
type DynamoDBTableAPI interface {
	DescribeTable(ctx context.Context, params *dynamodb.DescribeTableInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DescribeTableOutput, error)
	CreateTable(ctx context.Context, params *dynamodb.CreateTableInput, optFns ...func(*dynamodb.Options)) (*dynamodb.CreateTableOutput, error)
}

// EnsureTable verifica se a tabela DynamoDB existe e a cria caso não exista.
// Usa o modo de faturamento PAY_PER_REQUEST (sob demanda) e aguarda
// a tabela ficar ACTIVE antes de retornar (timeout de até 30s).
//
// O esquema da tabela corresponde ao modelo PaymentHistory:
//   - Chave de partição: payment_id (string)
//   - Chave de ordenação: timestamp  (string)
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
