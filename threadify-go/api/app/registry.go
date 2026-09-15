package app

import (
	"net/http"

	"github.com/gin-gonic/gin"
	sharedauth "threadify-go/shared/auth"
	"threadify-go/shared/registry"
)

// wrapRegistryManagement meters every API request, including invalid proxy
// requests; Engine skips duplicates only after verifying signed forwarding proof.
func wrapRegistryManagement(next http.Handler, runtime *registry.Runtime) http.Handler {
	return runtime.Wrap(next)
}

// licensedCompanyMiddleware prevents old local identities crossing the account binding.
func licensedCompanyMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if runtime := registry.Default(); runtime != nil {
			company, _ := c.Get(sharedauth.CtxCompanyID)
			companyID, _ := company.(string)
			if err := runtime.CheckCompany(companyID); err != nil {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "account is not authorized by the Threadify license"})
				return
			}
		}
		c.Next()
	}
}
