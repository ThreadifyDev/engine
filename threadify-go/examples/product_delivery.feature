Feature: product_delivery
Version: 1
Description: Complete product delivery workflow with retry limits.

Background:
  Given the thread must finish within "72h"
  And multiple terminal steps are allowed
  And multiple terminal severity is "major"
  And threads lock to this version

Rule: Record an order
  When step "order_placed" is submitted
  Then owner must be "merchant"
  And step type is "managed"
  And this step must finish within "5m"
  And content "order_id" must be present
  And content "customer_id" must be present
  And content "total_amount" must be present
  And content "notes" is optional
  And content "promo_code" is optional
  And this step is an entry point
  And next step must be one of "payment_validation"
  And the next step must start within "2m"
  And this step may be retried at most 3 times

Rule: Validate the payment
  When step "payment_validation" is submitted
  Then owner must be "payment_processor"
  And step type is "external"
  And this step must finish within "30s"
  And content "payment_method" must be present
  And content "amount" must be present
  And content "transaction_id" is optional
  And next step must be one of "payment_validated", "order_cancelled"
  And the next step must start within "30s"
  And this step may be retried at most 3 times

Rule: Confirm the payment
  When step "payment_validated" is submitted
  Then owner must be "payment_processor"
  And step type is "managed"
  And next step must be one of "fulfillment_ready"
  And the next step must start within "5m"

Rule: Cancel the order
  When step "order_cancelled" is submitted
  Then owner must be "merchant"
  And step type is "managed"
  And content "cancellation_reason" must be present
  And this step is terminal

Rule: Release the fulfilment
  When step "fulfillment_ready" is submitted
  Then owner must be "warehouse_manager"
  And step type is "human_in_loop"
  And this step must finish within "2h"
  And content "warehouse_location" must be present
  And content "items_count" must be present
  And next step must be one of "shipped"
  And the next step must start within "4h"

Rule: Ship the package
  When step "shipped" is submitted
  Then owner must be "logistics_carrier"
  And step type is "managed"
  And this step must finish within "24h"
  And content "tracking_number" must be present
  And content "carrier_name" must be present
  And content "estimated_delivery" is optional
  And next step must be one of "delivered", "delivery_failed"
  And the next step must start within "72h"

Rule: Deliver the package
  When step "delivered" is submitted
  Then owner must be "logistics_carrier"
  And step type is "managed"
  And content "delivery_timestamp" must be present
  And content "signature" must be present
  And this step is terminal

Rule: Record delivery failure
  When step "delivery_failed" is submitted
  Then owner must be "logistics_carrier"
  And step type is "managed"
  And content "failure_reason" must be present
  And next step must be one of "shipped"
  And the next step must start within "1h"
  And this step may be retried at most 2 times
