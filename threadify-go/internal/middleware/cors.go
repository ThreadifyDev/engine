package middleware

import (
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func CORSMiddleware(originsCSV string) gin.HandlerFunc {
	var origins []string
	if strings.TrimSpace(originsCSV) != "" {
		for _, o := range strings.Split(originsCSV, ",") {
			if trimmed := strings.TrimSpace(o); trimmed != "" {
				origins = append(origins, trimmed)
			}
		}
	}

	cfg := cors.DefaultConfig()
	if len(origins) > 0 {
		cfg.AllowOrigins = origins
	} else {
		cfg.AllowAllOrigins = true
	}

	cfg.AllowMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}
	cfg.AllowHeaders = []string{"Origin", "Content-Length", "Content-Type", "Authorization", "X-API-Key", "Accept"}
	cfg.ExposeHeaders = []string{"Content-Length"}
	cfg.MaxAge = 12 * time.Hour
	cfg.AllowCredentials = true

	return cors.New(cfg)
}
