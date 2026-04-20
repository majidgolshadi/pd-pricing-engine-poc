package dynamo

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"pricing-engine/internal/domain"
)

const tableName = "order_snapshots"

// SnapshotRepository implements domain.SnapshotRepository using DynamoDB.
type SnapshotRepository struct {
	client *dynamodb.Client
}

// NewSnapshotRepository creates a new DynamoDB-backed snapshot repository.
func NewSnapshotRepository(client *dynamodb.Client) *SnapshotRepository {
	return &SnapshotRepository{client: client}
}

func partitionKey(orderCode string) string {
	return "ORDER#" + orderCode
}

// Save persists a new snapshot, auto-incrementing the version number.
func (r *SnapshotRepository) Save(ctx context.Context, orderCode string, currency string, snapshot domain.PriceSnapshot) (*domain.OrderSnapshot, error) {
	// Determine next version by querying the latest
	nextVersion := 1
	latest, err := r.GetLatest(ctx, orderCode)
	if err != nil {
		return nil, fmt.Errorf("querying latest version: %w", err)
	}
	if latest != nil {
		nextVersion = latest.Version + 1
	}

	now := time.Now().UTC()

	// Serialize the PriceSnapshot to JSON and store as a string attribute.
	// This preserves the full nested structure without needing complex DynamoDB map marshalling.
	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		return nil, fmt.Errorf("marshalling snapshot: %w", err)
	}

	item := map[string]types.AttributeValue{
		"PK":           &types.AttributeValueMemberS{Value: partitionKey(orderCode)},
		"SK":           &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", nextVersion)},
		"OrderCode":    &types.AttributeValueMemberS{Value: orderCode},
		"Version":      &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", nextVersion)},
		"Currency":     &types.AttributeValueMemberS{Value: currency},
		"CreatedAt":    &types.AttributeValueMemberS{Value: now.Format(time.RFC3339)},
		"SnapshotData": &types.AttributeValueMemberS{Value: string(snapshotJSON)},
	}

	_, err = r.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(tableName),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(SK)"),
	})
	if err != nil {
		return nil, fmt.Errorf("putting item: %w", err)
	}

	return &domain.OrderSnapshot{
		OrderCode: orderCode,
		Version:   nextVersion,
		CreatedAt: now,
		Currency:  currency,
		Snapshot:  snapshot,
	}, nil
}

// GetAllVersions returns all snapshots for an order, sorted by version ascending.
func (r *SnapshotRepository) GetAllVersions(ctx context.Context, orderCode string) ([]domain.OrderSnapshot, error) {
	result, err := r.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(tableName),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: partitionKey(orderCode)},
		},
		ScanIndexForward: aws.Bool(true),
	})
	if err != nil {
		return nil, fmt.Errorf("querying all versions: %w", err)
	}

	return unmarshalItems(result.Items)
}

// GetByVersion returns a specific snapshot by order code and version.
func (r *SnapshotRepository) GetByVersion(ctx context.Context, orderCode string, version int) (*domain.OrderSnapshot, error) {
	result, err := r.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: partitionKey(orderCode)},
			"SK": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", version)},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("getting item: %w", err)
	}

	if result.Item == nil {
		return nil, nil
	}

	return unmarshalItem(result.Item)
}

// GetLatest returns the most recent snapshot for an order.
func (r *SnapshotRepository) GetLatest(ctx context.Context, orderCode string) (*domain.OrderSnapshot, error) {
	result, err := r.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(tableName),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: partitionKey(orderCode)},
		},
		ScanIndexForward: aws.Bool(false),
		Limit:            aws.Int32(1),
	})
	if err != nil {
		return nil, fmt.Errorf("querying latest: %w", err)
	}

	if len(result.Items) == 0 {
		return nil, nil
	}

	return unmarshalItem(result.Items[0])
}

// unmarshalItems converts a slice of DynamoDB items into OrderSnapshot values.
func unmarshalItems(items []map[string]types.AttributeValue) ([]domain.OrderSnapshot, error) {
	snapshots := make([]domain.OrderSnapshot, 0, len(items))
	for _, item := range items {
		s, err := unmarshalItem(item)
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, *s)
	}
	return snapshots, nil
}

// unmarshalItem converts a single DynamoDB item into an OrderSnapshot.
func unmarshalItem(item map[string]types.AttributeValue) (*domain.OrderSnapshot, error) {
	orderCode := ""
	if v, ok := item["OrderCode"].(*types.AttributeValueMemberS); ok {
		orderCode = v.Value
	}

	version := 0
	if v, ok := item["Version"].(*types.AttributeValueMemberN); ok {
		fmt.Sscanf(v.Value, "%d", &version)
	}

	currency := ""
	if v, ok := item["Currency"].(*types.AttributeValueMemberS); ok {
		currency = v.Value
	}

	var createdAt time.Time
	if v, ok := item["CreatedAt"].(*types.AttributeValueMemberS); ok {
		createdAt, _ = time.Parse(time.RFC3339, v.Value)
	}

	var snapshot domain.PriceSnapshot
	if v, ok := item["SnapshotData"].(*types.AttributeValueMemberS); ok {
		if err := json.Unmarshal([]byte(v.Value), &snapshot); err != nil {
			return nil, fmt.Errorf("unmarshalling snapshot data: %w", err)
		}
	}

	return &domain.OrderSnapshot{
		OrderCode: orderCode,
		Version:   version,
		CreatedAt: createdAt,
		Currency:  currency,
		Snapshot:  snapshot,
	}, nil
}