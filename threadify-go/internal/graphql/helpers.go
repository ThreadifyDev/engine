package graphql

import (
	"context"
	"fmt"
)

// getUserInfoFromContext extracts user info from GraphQL context
func getUserInfoFromContext(ctx context.Context) (ownerID, companyID, role string, err error) {
	ownerIDVal := ctx.Value("ownerID")
	companyIDVal := ctx.Value("companyID")
	roleVal := ctx.Value("role")

	if ownerIDVal == nil || companyIDVal == nil || roleVal == nil {
		return "", "", "", fmt.Errorf("user authentication context not found")
	}

	ownerID, ok1 := ownerIDVal.(string)
	companyID, ok2 := companyIDVal.(string)
	role, ok3 := roleVal.(string)

	if !ok1 || !ok2 || !ok3 {
		return "", "", "", fmt.Errorf("invalid user authentication context types")
	}

	return ownerID, companyID, role, nil
}
