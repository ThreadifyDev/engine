Feature: shipping_confirmation
Version: 1
Description: Match delivery to the latest successful shipment in this thread.

Rule: Record the shipment
  When step "order_shipped" is submitted
  Then owner must be "carrier"
  And content "tracking_number" must not be empty
  And this step is an entry point

Rule: Confirm delivery
  When step "delivery_confirmed" is submitted
  Then owner must be "carrier"
  And content "tracking_number" must equal order_shipped.tracking_number
  And this step is terminal
