Feature: order_review
Version: 1
Description: Review fraud and stock in parallel before fulfilment.

Rule: Receive the order
  When step "received" is submitted
  Then owner must be "sales"
  And content "order_id" must be present
  And this step is an entry point
  And next step must be one of "fraud_review", "stock_review"

Rule: Check fraud signals
  When step "fraud_review" is submitted
  Then owner must be "risk"
  And content "review_note" is optional
  And content "risk_reason" must satisfy question "Do the risk signals permit this order?"
  And semantic context for content "risk_reason" is "received"
  And semantic confidence for content "risk_reason" is 0.85
  And this step is terminal

Rule: Check stock
  When step "stock_review" is submitted
  Then owner must be "inventory"
  And this step is terminal

Group: "parallel_reviews"
  Given parallel steps are "fraud_review", "stock_review"
  And all parallel steps must succeed
  And combined duration must be within "5m"
