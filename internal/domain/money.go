package domain

// Money represents a monetary value in minor currency units (e.g., cents for EUR/USD).
//
// # Why Minor Units (int64)?
//
// All monetary values in this system are stored as int64 minor units rather than
// floating-point types. This avoids rounding errors inherent in float arithmetic
// and ensures correctness for financial operations such as:
//   - Tax calculation
//   - Discount allocation across items
//   - Reconciliation with payment providers
//   - Invoice and ledger consistency
//
// Examples of minor units:
//   - EUR: 599 = €5.99 (2 decimal places)
//   - JPY: 500 = ¥500 (0 decimal places)
//   - BHD: 1500 = 1.500 BHD (3 decimal places)
//
// The Currency field is an ISO 4217 code (e.g., "EUR", "USD", "JPY") and is carried
// alongside the amount for clarity. Currency mismatch validation is not enforced at
// the Money level — it is the responsibility of the caller to ensure consistency.
//
// # Usage in the Adjustment Model
//
// Money is used as the Amount field in Adjustment (see adjustment.go). Discounts use
// negative amounts, while fees and taxes use positive amounts. The final order total
// is computed as: subtotal + sum(all adjustment amounts).
type Money struct {
	Amount   int64  // Value in minor currency units (e.g., 599 = €5.99); negative for discounts
	Currency string // ISO 4217 currency code (e.g., "EUR", "USD", "JPY")
}

// NewMoney creates a Money value with the given amount (minor units) and currency code.
func NewMoney(amount int64, currency string) Money {
	return Money{Amount: amount, Currency: currency}
}

// Add returns a new Money whose Amount is the sum of m and other.
//
// Note: this method does NOT validate that both Money values share the same currency.
// The caller must ensure currency consistency. In this engine, all values within a
// single calculation share the cart's currency (set during NewContext).
func (m Money) Add(other Money) Money {
	return Money{Amount: m.Amount + other.Amount, Currency: m.Currency}
}