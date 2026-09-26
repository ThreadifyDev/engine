# Workflow contracts

Contracts define the permitted steps, owners, context fields, transitions, and terminal outcomes of a thread. Author them in Threadify's bounded Gherkin language and submit the `.feature` source as `text/plain`.

See the [complete Gherkin reference](../threadify-go/docs/GHERKIN_CONTRACTS.md) and the [product delivery example](../threadify-go/examples/product_delivery.feature).

## Example

```gherkin
Feature: payment_flow
Version: 1
Description: Review and settle a payment.

Background:
  Given the thread must finish within "1h"

Step: "requested"
  Description: "The merchant submitted a payment request."
  Required context: "payment_id" means "Merchant payment identifier."
  Required context: "amount" means "Amount requested."
  Optional context: "note" means "Additional payment details."
  Optional context: "merchant_reference" means "Merchant's external reference."

Step: "settled"
  Description: "The payment processor confirmed settlement."
  Required context: "settlement_id" means "Processor settlement identifier."

Rule: Receive the payment request
  When step "requested" is submitted
  Then owner must be "merchant"
  And content "payment_id" must be present
  And this step is an entry point
  And next step must be one of "settled"

Rule: Settle the payment
  When step "settled" is submitted
  Then owner must be "payment_processor"
  And content "settlement_id" must be present
  And this step is terminal
```

A `Step` block gives a step its business description and the context fields the Engine may use. Add one `Required context:` or `Optional context:` line per field, repeating either line type for multiple fields. `means "..."` is optional and helps classification. Required fields must be present; optional fields may be absent. Extra submitted context is accepted but is not used as contract facts. Duplicate or conflicting context declarations fail validation. A matching `Rule` defines ownership and executable checks.

Each step has one owner. `next step must be one of` declares immediate transitions. `step "name" must have succeeded` declares an earlier prerequisite without requiring an immediate transition. Background clauses set thread-wide duration, terminal severity, and version locking. Semantic question clauses define bounded classifier checks.

For two or more steps that may run in parallel, give each a `Rule` and use `Step` blocks to describe their business meaning and context. Then add a group using their step names:

```gherkin
Group: "parallel_reviews"
  Given parallel steps are "fraud_review", "stock_review"
  And all parallel steps must succeed
  And combined duration must be within "5m"
```

The first clause is required. The success and duration clauses are optional. Each name must refer to a step in the contract. A later step that needs both reviews should list both as prerequisites in its Rule.

## Publish and update

- `POST /v1/contracts/preview` validates source and returns a graph without saving.
- `POST /v1/contracts` creates version 1.
- `PUT /v1/contracts/{id}` publishes a new version after increasing `Version:`.
- `GET /v1/contracts/{id}/versions/{version}` returns the source and compiled graph.

Submitted contract source must start with `Feature:`. The Engine stores compiled rules as JSON and retains the authored Gherkin source for review.
