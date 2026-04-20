## Pricing / Calculation Engine (Ordering Industry)

### Overview / Problem Statement

Ordering systems (food delivery, e-commerce, retail) require a pricing engine that can calculate totals reliably while supporting constantly changing business rules such as promotions, coupons, delivery fees, and tax policies. The challenge is not only computing the final price, but doing so in a way that is **deterministic, auditable, configurable, and easy to extend** without risking regressions.

This pricing engine design is built to handle real-world complexity: multiple promotion types, item-level and order-level discounts, stacking/exclusivity logic, delivery overrides, tax calculation, and complete pricing breakdown.

---

## Key Design Principles

### Adjustment-Based Accounting Model

This pricing system uses an **adjustment-based accounting model** where ALL price-impacting operations are represented as explicit charge components ("adjustments"), rather than being embedded as implicit arithmetic inside totals.

The flow is:
1. The cart generates a base subtotal (items × quantity × unit price)
2. All subsequent price modifications (discounts, fees, taxes, rounding) are recorded as adjustments
3. The final order total is derived from: `base subtotal + sum(adjustments)`

Every pricing rule produces one or more adjustments, each with:
- **Type**: DISCOUNT / FEE / TAX / ROUNDING
- **Target**: ORDER or ITEM:\<SKU\>
- **Amount**: monetary delta (int64 minor units; discounts are negative)
- **Reason Code**: identifier such as promotion code, tax code, delivery fee rule
- **Description**: human-readable explanation
- **Metadata**: extensible attributes (promotion ID, campaign ID, rule version, tax category, etc.)
- **ID**: unique identifier for traceability

#### Why Adjustments?

1. **Auditability**: The system can always answer "Why is the total €18.40?" by enumerating adjustments.
2. **Extensibility**: New pricing features only require new adjustment producers, without changing persistence format.
3. **Correctness & Reconciliation**: Finance and reporting require exact breakdowns. Adjustments provide a stable representation for downstream billing, invoices, and analytics.
4. **Refunds**: Per-item adjustments make partial refunds straightforward — remove the cancelled item and its related adjustments, then recompute totals safely.
5. **Tax Compliance**: Per-item adjustments ensure correct taxable base per tax category (e.g., food vs alcohol vs service may have different VAT rates).

#### Item-Level vs Order-Level Adjustments

Multi-item promotions are typically "scattered" into multiple item-level adjustments for refund safety, tax compliance, invoice clarity, and reporting accuracy. Some discounts are truly order-level (e.g., "€5 off the entire order", "free delivery").

### Deterministic and Auditable

The engine must always produce the same result for the same input (cart + timestamp + config). Additionally, it returns a full breakdown of how the final total was produced.

### Extendable Rule Modeling

Business logic changes frequently. The engine is designed so new promotion types, conditions, and calculation logic can be added without rewriting core pricing logic.

### Separation of Concerns

Calculation is split into independent stages. Each stage has one responsibility, making the system easier to reason about and test.

---

## System Architecture (High-Level)

The pricing engine is implemented as a **pipeline-based calculation system**.
Each step is represented as a stage that mutates a shared calculation context.

**Input:** Cart (items, quantities, coupon, user/store metadata)  
**Output:** PriceSnapshot (subtotal, discounts, fees, taxes, rounding, total + full breakdown)

### Pipeline Flow

```
normalize → subtotal → apply_promotions → delivery_fee → tax → rounding → finalize
```

1. **NormalizeStage** — Validate and normalize cart (currency, items, quantities, prices)
2. **SubtotalStage** — Compute item line totals (qty × unit price) and order subtotal
3. **ApplyPromotionsStage** — Evaluate promotions (conditions + benefits) and emit discount adjustments
4. **DeliveryFeeStage** — Compute delivery/service fee adjustments (with free-delivery override support)
5. **TaxStage** — Compute tax based on adjusted totals
6. **RoundingStage** — Apply currency rounding rules and emit rounding adjustments
7. **FinalizeStage** — Aggregate all adjustments into final totals and validate the result

This pipeline design ensures modularity, testability, and deterministic execution.

---

## Core Data Model

### Money Representation

Money is represented using `int64` minor units (cents) rather than floating-point types. This avoids rounding errors and ensures correctness for financial operations.

- EUR: 599 = €5.99 (2 decimal places)
- JPY: 500 = ¥500 (0 decimal places)

### Adjustments-Based Accounting

Instead of directly modifying totals, the engine models all pricing effects as **Adjustments**:

* Discount adjustments (negative values)
* Fee adjustments (positive values)
* Tax adjustments (positive values)
* Rounding adjustments (±1 minor unit typically)

Adjustments are applied either at:

* **Item-level** (`ITEM:<sku>`)
* **Order-level** (`ORDER`)

This approach makes the engine explainable: the final price is simply the subtotal plus all adjustments.

### Price Snapshot Output

The output is a structured snapshot containing:

* Per-item final totals and adjustments
* Order-level adjustments (delivery, coupon, tax, rounding)
* Aggregated totals (subtotal, discounts, tax, fees, rounding, total)
* Engine version identifier for traceability
* Promotion execution trace (applied and skipped promotions with reasons)

This snapshot is persisted immutably at checkout to ensure the exact pricing can be reproduced later.

---

## Promotion System (Rule Engine)

Promotions are modeled as a combination of:

### Conditions

Conditions determine whether a promotion applies. Implementations:

* `MinSubtotalCondition` — minimum subtotal threshold
* `HasSKUCondition` — cart contains specific SKU

Conditions return both a boolean result and a reason string, enabling traceability.

### Benefits

Benefits generate adjustments when a promotion is applied. Implementations:

* `PercentOffOrderBenefit` — percentage off entire order → ORDER DISCOUNT
* `PercentOffSKUBenefit` — percentage off a specific SKU → ITEM DISCOUNT
* `BuyXGetYBenefit` — free items when quantity threshold met → ITEM DISCOUNT
* `FreeDeliveryBenefit` — signals delivery stage to waive fee → ORDER FEE override

### Stacking and Exclusivity

Promotions are sorted by priority and executed in deterministic order.
The engine supports:

* **Stackable promotions** — allows additional promotions after this one
* **Non-stackable** — stops evaluation after this promotion applies
* **Exclusive groups** — only one promotion in the same group may apply

### Promotion Trace

Every promotion evaluation (applied or skipped) is recorded with the reason, enabling debugging and customer support.

---

## Delivery Fee Handling

Delivery fee is treated as an order-level FEE adjustment. Promotions can override it via a metadata signal (`fee_override: "true"`). The delivery stage checks for this marker and resolves the final fee accordingly, keeping delivery logic decoupled from promotion logic.

---

## Tax Calculation Strategy

Tax is calculated after promotions and delivery are applied. The tax base is computed from:

* Final item totals (after item-level discounts)
* Delivery fee adjustments

Tax is then applied as a TAX adjustment, ensuring tax is based on what the customer actually pays.

---

## Rounding Mechanism

Rounding is a **first-class pricing operation**, not a UI formatting step. It is:

* Executed as a dedicated pipeline stage after discounts, fees, and tax
* Driven by configurable policy (method, scope, increment)
* Persisted as an explicit ROUNDING adjustment with full policy metadata

### Configurable Rounding Policy

* **Method**: HALF_UP, HALF_EVEN (banker's), FLOOR, CEIL
* **Scope**: ORDER_TOTAL, PER_ITEM, PER_TAX
* **Increment**: smallest unit to round to (1=cent, 5=5 cents, etc.)

---

## Immutable Snapshot Persistence

At checkout, the system persists a complete **PriceSnapshot** including:

* Per-item totals and adjustments
* Order-level adjustments (discounts, fees, taxes, rounding)
* Aggregated totals
* Engine version and promotion trace

Once persisted, this snapshot is **immutable** and becomes the source of truth for invoices, customer support, order history, and financial reconciliation.

**Historical orders should NEVER be recomputed using the latest pricing logic.**

### Versioning

Each order supports multiple snapshot versions (tracking price evolution as the cart changes). The repository auto-increments the version on each save.

Version metadata enables tracing exactly which code/config produced each result:
* Pricing engine version (e.g., "pricing-v2")
* Rounding policy ID and version (stored in adjustment metadata)

---

## Project Structure

```
├── main.go                          # Demo: configures pipeline, runs calculations, persists snapshots
├── internal/
│   ├── domain/                      # Core types (no external dependencies)
│   │   ├── adjustment.go            # Adjustment type and constants (DISCOUNT/FEE/TAX/ROUNDING)
│   │   ├── cart.go                  # Cart, LineItem, CouponInput
│   │   ├── money.go                 # Money (int64 minor units)
│   │   ├── promotion.go            # Promotion, Condition, Benefit interfaces
│   │   ├── rounding.go             # RoundingPolicy, RoundingMethod, ApplyRounding()
│   │   ├── snapshot.go             # PriceSnapshot, ItemSnapshot, OrderSnapshot, PromotionTrace
│   │   └── repository.go           # SnapshotRepository interface
│   ├── engine/                      # Pipeline engine
│   │   ├── engine.go               # Engine (orchestrates stage execution)
│   │   ├── stage.go                # Stage interface
│   │   ├── context.go              # CalcContext (shared mutable pipeline state)
│   │   └── trace.go                # PromoTrace, CalcTrace
│   ├── promos/                      # Promotion rule implementations
│   │   ├── benefits.go             # PercentOffOrder, PercentOffSKU, BuyXGetY, FreeDelivery
│   │   └── conditions.go           # MinSubtotal, HasSKU
│   ├── stages/                      # Pipeline stage implementations
│   │   ├── normalize.go            # Stage 1: cart validation
│   │   ├── subtotal.go             # Stage 2: line totals + subtotal
│   │   ├── apply_promotions.go     # Stage 3: promotion evaluation + discount adjustments
│   │   ├── delivery_fee.go         # Stage 4: delivery fee (with free-delivery override)
│   │   ├── tax.go                  # Stage 5: VAT/tax calculation
│   │   ├── rounding.go             # Stage 6: configurable rounding
│   │   └── finalize.go             # Stage 7: total aggregation + validation
│   └── infra/
│       └── dynamo/                  # DynamoDB persistence
│           ├── client.go           # LocalStack DynamoDB client
│           └── repository.go       # SnapshotRepository implementation
├── scripts/
│   └── init-aws.sh                 # LocalStack table creation script
├── docker-compose.yml               # LocalStack container
└── go.mod / go.sum                  # Go module dependencies
```

---

## How to Run

```bash
# Start LocalStack
docker-compose up -d

# Create DynamoDB table
./scripts/init-aws.sh

# Run the demo
go run main.go
```

---

## Extending the Engine

### Adding a New Promotion Type

1. Implement `domain.Condition` (if new eligibility logic is needed) in `internal/promos/conditions.go`
2. Implement `domain.Benefit` (for the new discount/fee logic) in `internal/promos/benefits.go`
3. Configure a `domain.Promotion` with the new condition/benefit — no engine changes required

### Adding a New Pipeline Stage

1. Create a new file in `internal/stages/` (e.g., `service_fee.go`)
2. Implement `engine.Stage` (Name + Execute)
3. Register the stage in the pipeline at the appropriate position in `main.go`

### Adding a New Persistence Backend

1. Implement `domain.SnapshotRepository` for your database
2. Replace the DynamoDB repository in `main.go`

---

## Example Calculation

Cart: burger (€5.99 × 2) + cola (€1.99 × 1)

| Step | Description | Amount |
|------|-------------|--------|
| Subtotal | 599×2 + 199×1 | 1397 |
| Item discount | 20% off burger: 1198 × 20% | -240 |
| Order discount | PROMO10 = 10% off subtotal: 1397 × 10% | -140 |
| Delivery fee | Base delivery | +299 |
| Service fee | Service charge | +150 |
| **Total** | 1397 - 240 - 140 + 449 | **1466** |

---

## Summary

This calculation engine is a modular, adjustment-driven pipeline that produces deterministic and auditable pricing snapshots. Promotions are configurable rule definitions (conditions + benefits) with built-in stacking and exclusivity. Delivery fees, taxes, and rounding are integrated as first-class adjustments, ensuring full transparency and correctness.

The result is a scalable architecture suitable for real-world ordering platforms, where pricing complexity grows rapidly and correctness is business-critical.