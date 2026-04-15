## Pricing / Calculation Engine (Ordering Industry)

### Overview / Problem Statement

Ordering systems (food delivery, e-commerce, retail) require a pricing engine that can calculate totals reliably while supporting constantly changing business rules such as promotions, coupons, delivery fees, and tax policies. The challenge is not only computing the final price, but doing so in a way that is **deterministic, auditable, configurable, and easy to extend** without risking regressions.

This pricing engine design is built to handle real-world complexity: multiple promotion types, item-level and order-level discounts, stacking/exclusivity logic, delivery overrides, tax calculation, and complete pricing breakdown.

---

## Key Design Principles

### Deterministic and Auditable

The engine must always produce the same result for the same input (cart + timestamp + config). Additionally, it must return a full breakdown of how the final total was produced (discount sources, fees, taxes). This is critical for financial reconciliation, dispute handling, and debugging.

### Extendable Rule Modeling

Business logic changes frequently. The engine is designed so new promotion types, conditions, and calculation logic can be added without rewriting core pricing logic.

### Separation of Concerns

Calculation is split into independent stages. Each stage has one responsibility (e.g., subtotal calculation, promotion application, tax calculation), making the system easier to reason about and test.

---

## System Architecture (High-Level)

The pricing engine is implemented as a **pipeline-based calculation system**.
Each step is represented as a stage that mutates a shared calculation context.

**Input:** Cart (items, quantities, coupon, user/store metadata)
**Output:** PriceSnapshot (subtotal, discounts, fees, taxes, total + full breakdown)

### Pipeline Flow

1. Normalize and validate cart
2. Compute item subtotals and order subtotal
3. Apply promotions (item-level and order-level)
4. Compute delivery fee (including free-delivery overrides)
5. Compute tax based on final adjusted totals
6. Finalize totals and validate results

This structure ensures calculation logic is modular, testable, and deterministic.

---

## Core Data Model

### Money Representation

Money is represented using `int64` minor units (cents) rather than floating-point types. This avoids rounding errors and ensures correctness for financial operations.

### Adjustments-Based Accounting

Instead of directly modifying totals, the engine models all pricing effects as **Adjustments**:

* Discount adjustments (negative values)
* Fee adjustments (positive values)
* Tax adjustments (positive values)

Adjustments are applied either at:

* **Item-level** (`ITEM:<sku>`)
* **Order-level** (`ORDER`)

This approach makes the engine explainable: the final price is simply the subtotal plus all adjustments.

### Price Snapshot Output

The output is a structured snapshot containing:

* Per-item final totals and adjustments
* Order-level adjustments (delivery, coupon, tax)
* Aggregated totals (subtotal, discounts, tax, fees, total)
* Engine version identifier for traceability

This snapshot is suitable for persistence at checkout to ensure the exact pricing can be reproduced later.

---

## Promotion System (Rule Engine Concept)

Promotions are modeled as a combination of:

### Conditions

Conditions determine whether a promotion applies. Examples:

* minimum subtotal
* cart contains specific SKU
* coupon present
* validity window

Conditions return both a boolean result and a reason string, enabling traceability.

### Benefits

Benefits generate adjustments when a promotion is applied. Supported benefit types include:

* percentage off entire order
* percentage off a specific SKU
* buy X get Y free
* free delivery override

### Stacking and Exclusivity

Promotions are sorted by priority and executed in deterministic order.
The engine supports:

* **stackable promotions**
* **exclusive groups**, where only one promotion in the same group can apply

This models real-world promo policies where multiple discounts cannot always combine.

---

## Delivery Fee Handling

Delivery fee is treated as an order-level fee adjustment.
Promotions can override it (e.g., free delivery coupon). Instead of hardcoding free delivery logic inside the delivery stage, the system detects an override adjustment and resolves the final fee accordingly.

This keeps delivery logic clean while still allowing business-controlled promo behavior.

---

## Tax Calculation Strategy

Tax is calculated after promotions and delivery are applied.
The tax base is computed from:

* final item totals (after item-level discounts)
* delivery fee adjustments

Tax is then applied as a tax adjustment. This ensures tax is based on what the customer actually pays, consistent with real-world VAT/GST models.

---

## Traceability and Explainability

The system records promotion execution traces, including:

* applied promotions
* skipped promotions and reasons (coupon missing, validity window, exclusivity conflict, unmet conditions)

This is critical for:

* debugging pricing disputes
* customer support explanations
* validating promotion configuration

---

## Operational Benefits

### Engineering Benefits

* Each stage is independently testable.
* Adding a new promotion type is isolated to implementing a new condition or benefit.
* Core engine code remains stable even as business rules evolve.

### Business Benefits

* Supports fast iteration on promotion strategies.
* Provides detailed audit logs for finance and compliance.
* Allows safe rollout of new pricing logic with versioning.

---

## Summary

This calculation engine is designed as a modular, adjustment-driven pipeline that produces deterministic and auditable pricing snapshots. Promotions are treated as configurable rule definitions (conditions + benefits) with built-in stacking and exclusivity. Delivery fees and taxes are integrated as first-class adjustments, ensuring full transparency and correctness.

The result is a scalable architecture suitable for real-world ordering platforms, where pricing complexity grows rapidly and correctness is business-critical.
