Feature: payment_processing
Version: 1
Description: Require approval and valid payment details before recording a charge.

Background:
  Given the thread must finish within "1h"

Rule: Approval must be recorded by the reviewer
  When step "approval" is submitted
  Then owner must be "reviewer"
  And this step is an entry point
  And content "reference" must not be empty

Rule: A charge requires approval and valid payment details
  When step "charge" is submitted
  Then owner must be "payment_processor"
  And step "approval" must have succeeded
  And content "amount" must be a number greater than 0
  And content "currency" must be one of "GBP", "USD", "EUR"
  And content "reference" must be present
  And this step must finish within "30s"
  And this step is terminal
