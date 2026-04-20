// Package dynamo provides a DynamoDB-backed implementation of domain.SnapshotRepository.
//
// # Persistence Layer
//
// This package handles the persistence of order pricing snapshots (domain.OrderSnapshot)
// to DynamoDB. It implements the domain.SnapshotRepository interface, allowing the
// pricing engine's output to be stored immutably for:
//   - Invoice generation
//   - Customer support and order history
//   - Financial reconciliation
//   - Audit and compliance
//
// # DynamoDB Table Design
//
// Table name: "order_snapshots"
// Key schema:
//   - PK (Partition Key, String): "ORDER#<orderCode>" — groups all versions of an order
//   - SK (Sort Key, Number): version number — enables ordered retrieval and latest queries
//
// The PriceSnapshot is serialized as JSON and stored in a "SnapshotData" string attribute,
// preserving the full nested structure without complex DynamoDB map marshalling.
//
// # Local Development
//
// For local development, this package connects to LocalStack (a local AWS emulator)
// using static test credentials. The DynamoDB table is created by the init script
// (see scripts/init-aws.sh).
package dynamo

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

// NewLocalClient creates a DynamoDB client configured for LocalStack (local development).
//
// It uses static test credentials ("test"/"test"/"test") and connects to the specified
// endpoint URL (typically "http://localhost:4566" for LocalStack).
//
// For production use, replace this with a client using proper AWS credentials and
// configuration (e.g., IAM roles, environment-based config).
func NewLocalClient(ctx context.Context, endpoint string) (*dynamodb.Client, error) {
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "test")),
	)
	if err != nil {
		return nil, err
	}

	client := dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})

	return client, nil
}