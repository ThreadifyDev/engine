package auth

const (
	CtxUserID = "userID" // Internal Threadify user ID

	CtxAuthUserID = "authUserID" // Supabase auth_user_id (for RBAC)

	CtxCompanyID = "companyID"

	CtxEmail = "email"

	CtxRoles = "roles"

	CtxAuthSub = "authSub"

	CtxClaims = "claims"

	CtxOwnerID = "ownerId"
)
