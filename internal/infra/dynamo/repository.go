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

// tableName is the DynamoDB table used for storing order pricing snapshots.
//
// Table schema:
//   - PK (String): partition key, formatted as "ORDER#<orderCode>"
//   - SK (Number): sort key, the auto-incremented version number
//
// Additional attributes:
//   - OrderCode (String): raw order code
//   - Version (Number): version number (same as SK)
//   - Currency (String): ISO 4217 currency code
//   - CreatedAt (String): RFC3339 timestamp
//   - SnapshotData (String): JSON-serialized domain.PriceSnapshot
//
// The table is created by scripts/init-aws.sh for LocalStack development.
const tableName = "order_snapshots"

// SnapshotRepository implements domain.SnapshotRepository using DynamoDB.
//
// # Immutable Snapshot Persistence
//
// Once a PriceSnapshot is computed by the pricing engine, it is persisted here as an
// immutable record. This snapshot becomes the source of truth for:
//   - Invoice generation
//   - Customer support explanations
//   - Order history display
//   - Financial reconciliation
//   - Audit and compliance
//
// Historical orders should NEVER be recomputed using the latest pricing logic.
//
// # Versioning
//
// Each order can have multiple snapshot versions. Save() auto-increments the version
// by querying the latest existing version. This allows tracking price evolution as
// the cart is modified before final checkout.
//
// # Data Format
//
// The PriceSnapshot is serialized to JSON and stored as a string attribute
// ("SnapshotData"), preserving the full nested structure (items, adjustments, traces).
//
// # Concurrency
//
// Save() uses a condition expression (attribute_not_exists(SK)) to prevent overwriting.
// In high-concurrency scenarios, callers should handle conditional check failures with retry.
type SnapshotRepository struct {
	client *dynamodb.Client
}

// NewSnapshotRepository creates a new DynamoDB-backed snapshot repository.
func NewSnapshotRepository(client *dynamodb.Client) *SnapshotRepository {
	return &SnapshotRepository{client: client}
}

// partitionKey formats the DynamoDB partition key for a given order code.
// Format: "ORDER#<orderCode>" — groups all versions of an order under one partition.
func partitionKey(orderCode string) string {
	return "ORDER#" + orderCode
}

// Save persists a new snapshot, auto-incrementing the version number.
//
// Algorithm:
//  1. Query the latest existing version for this order (GetLatest)
//  2. Set nextVersion = latestVersion + 1 (or 1 if no versions exist)
//  3. Serialize the PriceSnapshot to JSON
//  4. Write to DynamoDB with a condition to prevent version conflicts
//
// Returns the created OrderSnapshot with the assigned version number.
func (r *SnapshotRepository) Save(ctx context.Context, orderCode string, currency string, snapshot domain.PriceSnapshot) (*domain.OrderSnapshot, error) {
	// Determine next version by querying the latest existing snapshot.
	nextVersion := 1
	latest, err := r.GetLatest(ctx, orderCode)
	if err != nil {
		return nil, fmt.Errorf("querying latest version: %w", err)
	}
	if latest != nil {
		nextVersion = latest.Version + 1
	}

	now := time.Now().UTC()

	// Serialize the PriceSnapshot to JSON for storage as a string attribute.
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

	// Condition prevents overwriting an existing version (optimistic concurrency).
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
//
// Uses a DynamoDB Query with ScanIndexForward=true to return versions in ascending order.
// This is useful for viewing the price evolution of an order over time.
func (r *SnapshotRepository) GetAllVersions(ctx context.Context, orderCode string) ([]domain.OrderSnapshot, error) {
	result, err := r.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(tableName),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: partitionKey(orderCode)},
		},
		ScanIndexForward: aws.Bool(true), // ascending version order
	})
	if err != nil {
		return nil, fmt.Errorf("querying all versions: %w", err)
	}

	return unmarshalItems(result.Items)
}

// GetByVersion returns a specific snapshot by order code and version.
//
// Uses a DynamoDB GetItem with the composite key (PK + SK).
// Returns nil (not an error) if the snapshot doesn't exist.
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
//
// Uses a DynamoDB Query with ScanIndexForward=false and Limit=1 to efficiently
// retrieve only the highest version number. Returns nil if no snapshots exist.
func (r *SnapshotRepository) GetLatest(ctx context.Context, orderCode string) (*domain.OrderSnapshot, error) {
	result, err := r.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(tableName),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: partitionKey(orderCode)},
		},
		ScanIndexForward: aws.Bool(false), // descending — latest first
		Limit:            aws.Int32(1),     // only need the most recent
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
//
// It extracts scalar attributes (OrderCode, Version, Currency, CreatedAt) and
// deserializes the SnapshotData JSON string back into a domain.PriceSnapshot.
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

	// Deserialize the JSON-encoded PriceSnapshot from the SnapshotData attribute.
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