package validator

import (
	"os"
	"strings"
	"testing"
)

func TestGherkinParallelGroupAndSemanticRule(t *testing.T) {
	source := `Feature: order_review
Version: 1

Rule: Receive order
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
`
	c, result := NewContractValidator().Validate(source)
	if !result.IsValid {
		t.Fatalf("invalid: %+v", result.Errors)
	}
	if len(c.Groups) != 1 || len(c.Groups[0].Steps) != 2 || c.Groups[0].Rules.AllMustSucceed == nil || !*c.Groups[0].Rules.AllMustSucceed {
		t.Fatalf("group lost: %+v", c.Groups)
	}
	rule := c.Steps[1].SemanticRules[0]
	if rule.Field != "risk_reason" || rule.MinProbability != .85 || len(rule.ContextSteps) != 1 || rule.ContextSteps[0] != "received" {
		t.Fatalf("semantic rule lost: %+v", rule)
	}
	if len(c.Steps[1].BusinessContext.Optional) != 1 || c.Steps[1].BusinessContext.Optional[0] != "review_note" {
		t.Fatalf("optional context lost: %+v", c.Steps[1].BusinessContext)
	}
}

func TestPublishedGherkinExamplesValidate(t *testing.T) {
	for _, name := range []string{"payment.feature", "product_delivery.feature", "inventory_delivery_v3.feature", "parallel_review.feature"} {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile("../../examples/" + name)
			if err != nil {
				t.Fatal(err)
			}
			_, result := NewContractValidator().Validate(string(data))
			if !result.IsValid {
				t.Fatalf("invalid example: %+v", result.Errors)
			}
		})
	}
}

func TestContractValidatorRejectsYamlSource(t *testing.T) {
	_, result := NewContractValidator().Validate("contract_name: old_contract\nversion: 1\nsteps: []\n")
	if result.IsValid || len(result.Errors) == 0 {
		t.Fatalf("YAML source must be rejected: %+v", result)
	}
}

func TestGherkinStepDefinitionCompilesBusinessMeaningAndContext(t *testing.T) {
	source := `Feature: payment_processing
Step: "charge"
  Description: "The provider accepted the payment charge."
  Required context: "amount" means "Amount accepted in pounds."
  Optional context: "provider_reference" means "Provider's receipt identifier."

Rule: Charge a payment
  When step "charge" is submitted
  Then owner must be "processor"
  And content "amount" must be a number greater than 0
  And this step is an entry point
  And this step is terminal
`
	contract, result := NewContractValidator().Validate(source)
	if !result.IsValid {
		t.Fatalf("invalid Step definition: %+v", result.Errors)
	}
	step := contract.Steps[0]
	if step.Description != "The provider accepted the payment charge." {
		t.Fatalf("description: %q", step.Description)
	}
	if len(step.BusinessContext.Required) != 1 || step.BusinessContext.Required[0] != "amount" {
		t.Fatalf("required context: %+v", step.BusinessContext)
	}
	if len(step.BusinessContext.Optional) != 1 || step.BusinessContext.Optional[0] != "provider_reference" {
		t.Fatalf("optional context: %+v", step.BusinessContext)
	}
	if step.BusinessContext.Descriptions["provider_reference"] != "Provider's receipt identifier." {
		t.Fatalf("context meanings: %+v", step.BusinessContext.Descriptions)
	}
}

func TestGherkinStepDefinitionValidation(t *testing.T) {
	base := `Feature: review
Step: "reviewed"
  Description: "Review the application."
  Optional context: "note"

Rule: Review
  When step "reviewed" is submitted
  Then owner must be "risk"
  And this step is an entry point
  And this step is terminal
`
	for _, tc := range []struct{ name, source string }{
		{"missing description", strings.Replace(base, "  Description: \"Review the application.\"\n", "", 1)},
		{"duplicate Step description", strings.Replace(base, "  Optional context: \"note\"", "  Description: \"Review it again.\"\n  Optional context: \"note\"", 1)},
		{"missing Rule", `Feature: review
Step: "reviewed"
  Description: "Review the application."
`},
		{"duplicate definition", strings.Replace(base, "Rule: Review", "Step: \"reviewed\"\n  Description: \"Again.\"\nRule: Review", 1)},
		{"conflicting context", strings.Replace(base, "  Optional context: \"note\"", "  Optional context: \"note\"\n  Required context: \"note\"", 1)},
		{"conflicting Rule context", strings.Replace(base, "  And this step is an entry point", "  And content \"note\" must be present\n  And this step is an entry point", 1)},
		{"duplicate description", strings.Replace(base, "  And this step is an entry point", "  And step description is \"Other meaning.\"\n  And this step is an entry point", 1)},
		{"long context meaning", strings.Replace(base, "  Optional context: \"note\"", "  Optional context: \"note\" means \""+strings.Repeat("x", 501)+"\"", 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, result := NewContractValidator().Validate(tc.source)
			if result.IsValid || len(result.Errors) == 0 {
				t.Fatalf("invalid Step definition accepted: %s", tc.source)
			}
		})
	}
}

func TestGherkinLegacyStepDefinitionStillValidates(t *testing.T) {
	source := `Feature: review
Step: "reviewed"
  Given description is "The reviewer approved the application."
  And optional context "note" means "Reviewer note."
Rule: Review
  When step "reviewed" is submitted
  Then owner must be "risk"
  And this step is an entry point
  And this step is terminal
`
	contract, result := NewContractValidator().Validate(source)
	if !result.IsValid {
		t.Fatalf("legacy Step definition invalid: %+v", result.Errors)
	}
	if got := contract.Steps[0].BusinessContext.Descriptions["note"]; got != "Reviewer note." {
		t.Fatalf("context meaning = %q", got)
	}
}

func TestGherkinAdvancedClausesRejectInvalidDefinitions(t *testing.T) {
	base := `Feature: review
Rule: Receive
  When step "received" is submitted
  Then owner must be "sales"
  And this step is an entry point
  And next step must be one of "reviewed"
Rule: Review
  When step "reviewed" is submitted
  Then owner must be "risk"
  And this step is terminal
`
	for _, tc := range []struct{ name, suffix string }{
		{"missing semantic question", "  And semantic context for content \"reason\" is \"received\"\n"},
		{"invalid semantic probability", "  And content \"reason\" must satisfy question \"Is this acceptable?\"\n  And semantic confidence for content \"reason\" is 0.2\n"},
		{"unknown semantic context step", "  And content \"reason\" must satisfy question \"Is this acceptable?\"\n  And semantic context for content \"reason\" is \"missing\"\n"},
		{"unknown group member", "Group: \"checks\"\n  Given parallel steps are \"received\", \"missing\"\n"},
		{"single group member", "Group: \"checks\"\n  Given parallel steps are \"received\"\n"},
		{"invalid group duration", "Group: \"checks\"\n  Given parallel steps are \"received\", \"reviewed\"\n  And combined duration must be within \"soon\"\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := base + tc.suffix
			if tc.name == "missing semantic question" || tc.name == "invalid semantic probability" || tc.name == "unknown semantic context step" {
				source = base[:len(base)-len("  And this step is terminal\n")] + tc.suffix + "  And this step is terminal\n"
			}
			_, result := NewContractValidator().Validate(source)
			if result.IsValid || len(result.Errors) == 0 {
				t.Fatalf("invalid definition accepted: %s", source)
			}
		})
	}
}
