package app

import (
	_ "embed"
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	sharedauth "threadify-go/shared/auth"
	mh "threadify-go/shared/management/handlers"
	mr "threadify-go/shared/management/repository"
	ms "threadify-go/shared/management/service"
	"threadify-go/shared/rbac"
	"threadify-go/shared/registry"
	sharedrepo "threadify-go/shared/repository"
)

//go:embed code_samples.json
var managementCodeSamples []byte

// Mount the management services directly against the Engine's PostgreSQL pool.
// Authentication, license binding, CSRF and egress are supplied by the parent router.
func mountManagementRoutes(v1 *gin.RouterGroup, inf *infra, repos *repositories, roles *rbac.Loader, logger *zap.Logger) {
	users := mr.NewUserRepository(inf.db.Pool)
	companies := mr.NewCompanyRepository(inf.db.Pool)
	userRoles := mr.NewUserRoleRepository(inf.db.Pool)
	accounts := mr.NewServiceAccountRepository(inf.db.Pool)
	keys := mr.NewAPIKeyRepository(inf.db.Pool)
	profileView := mh.NewProfileViewHandler(sharedrepo.NewProfileViewRepository(inf.db.Pool), roles)
	profile := mh.NewEntityProfileTypeHandler(ms.NewEntityProfileTypeService(repos.entityProfileType, logger))
	user := mh.NewUserHandler(ms.NewUserService(users, companies, userRoles, nil, nil, nil, logger))
	account := mh.NewServiceAccountHandler(ms.NewServiceAccountService(logger, accounts, userRoles), roles)
	key := mh.NewAPIKeyHandler(ms.NewAPIKeyService(keys, accounts, userRoles, managementRoleNames{roles}, logger))
	role := mh.NewRoleHandler(roles)
	guard := func(permission string) gin.HandlerFunc { return managementPermission(roles, permission) }
	v1.GET("/code-samples", func(c *gin.Context) {
		var samples map[string]map[string]string
		if err := json.Unmarshal(managementCodeSamples, &samples); err != nil {
			c.JSON(500, gin.H{"error": "code_samples_unavailable"})
			return
		}
		kind := c.DefaultQuery("codeType", "basic_instrumentation")
		value, ok := samples[kind]
		if !ok {
			c.JSON(404, gin.H{"error": "unknown_code_type"})
			return
		}
		c.JSON(200, gin.H{"code_type": kind, "samples": value})
	})
	v1.GET("/roles", role.GetRoles)
	v1.GET("/roles/:level", role.GetRolesByLevel)
	v1.GET("/entity-profile-views/:id", profileView.Get)
	v1.PUT("/entity-profile-views/:id", profileView.Save)
	v1.GET("/entity-profile-types", guard("entity_profile_type.read"), profile.ListEntityProfileTypes)
	v1.PUT("/entity-profile-types/:slug", guard("entity_profile_type.update"), profile.ApplyEntityProfileType)
	v1.POST("/entity-profile-types/:slug/rename", guard("entity_profile_type.update"), profile.RenameEntityProfileType)
	v1.DELETE("/entity-profile-types/:slug", guard("entity_profile_type.delete"), profile.ArchiveEntityProfileType)
	v1.GET("/metrics-templates", guard("metrics_template.read"), profile.ListMetricsTemplates)
	v1.GET("/api-keys", guard("apikey.read"), key.ListAPIKeys)
	v1.POST("/api-keys", guard("apikey.create"), key.CreateAPIKey)
	v1.DELETE("/api-keys/:id", guard("apikey.delete"), key.RevokeAPIKey)
	v1.GET("/service-accounts", guard("serviceaccount.read"), account.ListServiceAccounts)
	v1.POST("/service-accounts", guard("serviceaccount.create"), account.CreateServiceAccount)
	v1.GET("/service-accounts/:id", guard("serviceaccount.read"), account.GetServiceAccount)
	v1.PUT("/service-accounts/:id", guard("serviceaccount.update"), account.UpdateServiceAccount)
	v1.DELETE("/service-accounts/:id", guard("serviceaccount.delete"), account.DeleteServiceAccount)
	v1.GET("/service-accounts/scopes/:scope/permissions", guard("serviceaccount.read"), account.GetPermissions)
	// Only a human principal can edit a personal/company profile.
	human := func(c *gin.Context) {
		var exists bool
		err := inf.db.Pool.QueryRow(c.Request.Context(), "SELECT EXISTS(SELECT 1 FROM users WHERE id=$1 AND company_id=$2)", c.GetString(sharedauth.CtxUserID), c.GetString(sharedauth.CtxCompanyID)).Scan(&exists)
		if err != nil {
			c.AbortWithStatusJSON(500, gin.H{"error": "profile_unavailable"})
			return
		}
		if !exists {
			c.AbortWithStatusJSON(403, gin.H{"error": "human_profile_required"})
			return
		}
		c.Next()
	}
	v1.GET("/user/profile", human, user.GetProfile)
	v1.POST("/user/profile", human, user.UpdateProfile)
	v1.POST("/user/mark-instrumentation-done", human, user.MarkInstrumentationDone)
	v1.GET("/billing/plan", func(c *gin.Context) {
		snapshot, err := registry.Default().Snapshot()
		if err != nil {
			c.JSON(registry.StatusCode(err), gin.H{"error": err.Error()})
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"billing_source": "registry", "account_id": snapshot.AccountID, "entitlements": snapshot.Entitlements})
	})
}

func managementPermission(loader *rbac.Loader, permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		names, _ := c.Get(sharedauth.CtxRoles)
		assigned, _ := names.([]string)
		for _, level := range []string{"app_level", "api_level"} {
			roles := loader.GetRolesByLevel(level)
			for _, name := range assigned {
				if role, ok := roles[name]; ok && loader.CheckPermission(role.Permissions, permission) {
					c.Next()
					return
				}
			}
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions", "required": permission})
	}
}

// Key creation accepts role names only; permission checks retain full definitions.
type managementRoleNames struct{ loader *rbac.Loader }

func (r managementRoleNames) GetRolesByLevel(level string) map[string]struct{} {
	names := make(map[string]struct{})
	for name := range r.loader.GetRolesByLevel(level) {
		names[name] = struct{}{}
	}
	return names
}
