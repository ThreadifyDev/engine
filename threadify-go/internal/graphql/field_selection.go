package graphql

import (
	"context"

	"github.com/99designs/gqlgen/graphql"
)

// FieldSelection represents the fields requested in a GraphQL query
type FieldSelection struct {
	fields map[string]bool
}

// NewFieldSelection creates a new field selection tracker
func NewFieldSelection() *FieldSelection {
	return &FieldSelection{
		fields: make(map[string]bool),
	}
}

// Has checks if a field was requested in the GraphQL query
func (fs *FieldSelection) Has(fieldName string) bool {
	return fs.fields[fieldName]
}

// Add marks a field as requested
func (fs *FieldSelection) Add(fieldName string) {
	fs.fields[fieldName] = true
}

// ExtractFieldSelections extracts the fields requested in the current GraphQL query
// This allows us to only load data that was actually requested
func ExtractFieldSelections(ctx context.Context) *FieldSelection {
	selection := NewFieldSelection()

	// Get the field context from gqlgen
	fieldCtx := graphql.GetFieldContext(ctx)
	if fieldCtx == nil {
		return selection
	}

	// Collect all requested fields recursively
	var collectSelections func(selections []graphql.CollectedField)
	collectSelections = func(selections []graphql.CollectedField) {
		for _, field := range selections {
			selection.Add(field.Name)
			if field.Selections != nil {
				// We need to pass satisfiers, but for simple field names we can just pass empty string slice
				childSelections := graphql.CollectFields(graphql.GetOperationContext(ctx), field.Selections, nil)
				collectSelections(childSelections)
			}
		}
	}

	collectSelections(graphql.CollectFieldsCtx(ctx, nil))

	return selection
}
