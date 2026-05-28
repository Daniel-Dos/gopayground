package provider

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

// DynamoDBConfig contém parâmetros de conexão para o cliente DynamoDB.
type DynamoDBConfig struct {
	Endpoint  string
	Region    string
	AccessKey string
	SecretKey string
}

// NewDynamoDBClient cria um cliente DynamoDB configurado centralizadamente.
// Detecta automaticamente endpoints locais (LocalStack/Floci) para usar
// credenciais estáticas, evitando dependência de IMDS da AWS.
// Para endpoints remotos (produção), usa a cadeia de credenciais padrão da AWS.
func NewDynamoDBClient(ctx context.Context, cfg DynamoDBConfig) (*dynamodb.Client, error) {
	var loadOpts []func(*awsconfig.LoadOptions) error

	if isLocalEndpoint(cfg.Endpoint) {
		region := cfg.Region
		if region == "" {
			region = "us-east-1"
		}

		accessKey := cfg.AccessKey
		secretKey := cfg.SecretKey
		if accessKey == "" || secretKey == "" {
			accessKey = "test"
			secretKey = "test"
		}

		slog.Info("usando credenciais AWS estáticas para endpoint DynamoDB local",
			"endpoint", cfg.Endpoint, "region", region)
		loadOpts = append(loadOpts,
			awsconfig.WithCredentialsProvider(
				credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
			),
			awsconfig.WithRegion(region),
		)
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("carregar config AWS: %w", err)
	}

	return dynamodb.NewFromConfig(awsCfg, func(o *dynamodb.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
	}), nil
}

// isLocalEndpoint verifica se o endpoint DynamoDB aponta para um contêiner
// local (LocalStack, Floci, localhost) que aceita credenciais estáticas.
func isLocalEndpoint(endpoint string) bool {
	lower := strings.ToLower(endpoint)
	return strings.Contains(lower, "localhost") ||
		strings.Contains(lower, "127.0.0.1") ||
		strings.Contains(lower, "floci") ||
		strings.Contains(lower, "localstack")
}
