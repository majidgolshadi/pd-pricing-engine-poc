package domain

import "context"

// SnapshotRepository defines persistence operations for order pricing snapshots.
//
// # Immutable Snapshot Persistence
//
// Once a PriceSnapshot is computed by the pricing engine, it must be persisted as an
// immutable record. This persisted snapshot becomes the source of truth for:
//   - Invoice generation
//   - Customer support explanations ("Why is the total €18.40?")
//   - Order history display
//   - Financial reconciliation with payment providers
//   - Audit and compliance
//
// Historical orders should NEVER be recomputed using the latest pricing logic.
// Instead, the persisted snapshot is the authoritative record of what was calculated
// at the time of checkout.
//
// # Versioning
//
// The repository supports multiple versions per order (OrderCode + Version).
// Each call to Save() auto-increments the version number. This enables:
//   - Tracking price evolution as the cart is modified before checkout
//   - Debugging by comparing snapshots across versions
//   - Safe rollback to a previous pricing state
//
// # Implementation
//
// The current implementation uses DynamoDB (see internal/infra/dynamo/repository.go),
// with OrderCode as the partition key and Version as the sort key. The PriceSnapshot
// is serialized as JSON and stored as a string attribute, preserving the full nested
// structure without complex DynamoDB map marshalling.
//
// To add a new persistence backend (e.g., PostgreSQL, MongoDB), implement this interface.
type SnapshotRepository interface {
	// Save persists a new snapshot for the given order code.
	// It auto-increments the version number by querying the latest existing version.
	// Returns the created OrderSnapshot with the assigned version number, or an error.
	Save(ctx context.Context, orderCode string, currency string, snapshot PriceSnapshot) (*OrderSnapshot, error)

	// GetAllVersions returns all snapshots for a given order code, ordered by version ascending.
	// Useful for viewing the price evolution of an order over time.
	GetAllVersions(ctx context.Context, orderCode string) ([]OrderSnapshot, error)

	// GetByVersion returns a specific snapshot identified by order code and version.
	// Returns nil if not found (no error). Useful for debugging a specific calculation.
	GetByVersion(ctx context.Context, orderCode string, version int) (*OrderSnapshot, error)

	// GetLatest returns the most recent snapshot for a given order code.
	// Returns nil if no snapshots exist for the order (no error).
	// This is typically the snapshot used for checkout/payment.
	GetLatest(ctx context.Context, orderCode string) (*OrderSnapshot, error)
}