package validator

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOptionalBusinessContextAndDeclaredRuleFields(t *testing.T) {
	base := `Feature: checkout
Version: 1
Description: Checkout workflow
Rule: Authorize payment
  When step "payment_authorized" is submitted
  Then owner must be "merchant"
  And content "order_id" must be present
  And content "provider_id" is optional
`
	_, result := NewContractValidator().Validate(base)
	require.True(t, result.IsValid, "%+v", result.Errors)

	_, result = NewContractValidator().Validate(base + `  And content "provider_id" must not be empty
`)
	require.False(t, result.IsValid)
	require.Contains(t, result.Errors[0].Message, "optional")
}
