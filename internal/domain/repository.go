package domain

import "context"

// SnapshotRepository defines persistence operations for order pricing snapshots.
type SnapshotRepository interface {
	// Save persists a new snapshot for the given order code.
	// It auto-increments the version and returns the assigned version number.
	Save(ctx context.Context, orderCode string, currency string, snapshot PriceSnapshot) (*OrderSnapshot, error)

	// GetAllVersions returns all snapshots for a given order code, ordered by version ascending.
	GetAllVersions(ctx context.Context, orderCode string) ([]OrderSnapshot, error)

	// GetByVersion returns a specific snapshot identified by order code and version.
	GetByVersion(ctx context.Context, orderCode string, version int) (*OrderSnapshot, error)

	// GetLatest returns the most recent snapshot for a given order code.
	GetLatest(ctx context.Context, orderCode string) (*OrderSnapshot, error)
}