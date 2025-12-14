package handlers

import (
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/threadify/engine/internal/middleware"
	"github.com/threadify/engine/internal/service"
)

type ContractHandler struct {
	contractService *service.ContractService
	authService     *service.AuthService
}

func NewContractHandler(contractService *service.ContractService, authService *service.AuthService) *ContractHandler {
	return &ContractHandler{
		contractService: contractService,
		authService:     authService,
	}
}

func (h *ContractHandler) GetAllContracts(c *gin.Context) {
	claimsInterface := c.MustGet("claims")
	claims, ok := claimsInterface.(jwt.MapClaims)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid claims format"})
		return
	}
	ownerID, ok := claims["ownerId"].(string)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid ownerId in claims"})
		return
	}

	statusCode, response := h.contractService.GetAllContracts(c.Request.Context(), ownerID)
	c.JSON(statusCode, response)
}

func (h *ContractHandler) Login(c *gin.Context) {
	var req struct {
		UserID string `json:"userId"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	token, err := h.authService.CreateToken(req.UserID, map[string]interface{}{
		"role":    "user",
		"ownerId": req.UserID,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token":   token,
		"userId":  req.UserID,
		"message": "Use this token in Authorization header as: Bearer <token>",
	})
}

func (h *ContractHandler) CreateContract(c *gin.Context) {
	userID := c.GetString("userID")
	claimsInterface := c.MustGet("claims")
	claims, ok := claimsInterface.(jwt.MapClaims)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid claims format"})
		return
	}
	ownerID, ok := claims["ownerId"].(string)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid ownerId in claims"})
		return
	}

	// Read YAML content from request body
	yamlContent, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
		return
	}

	statusCode, response := h.contractService.CreateContract(c.Request.Context(), ownerID, userID, string(yamlContent))

	// Record metrics
	if statusCode == 200 {
		middleware.RecordContractValidation(true)
		middleware.RecordContractVersionCreated()
	} else {
		middleware.RecordContractValidation(false)
	}

	c.JSON(statusCode, response)
}

func (h *ContractHandler) GetContract(c *gin.Context) {
	contractID := c.Param("id")
	claimsInterface := c.MustGet("claims")
	claims, ok := claimsInterface.(jwt.MapClaims)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid claims format"})
		return
	}
	requesterID, ok := claims["ownerId"].(string)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid ownerId in claims"})
		return
	}

	var version *int
	if v := c.Query("version"); v != "" {
		ver, err := strconv.Atoi(v)
		if err == nil {
			version = &ver
		}
	}

	statusCode, response := h.contractService.GetContract(c.Request.Context(), contractID, requesterID, version)
	c.JSON(statusCode, response)
}

func (h *ContractHandler) UpdateContract(c *gin.Context) {
	contractID := c.Param("id")
	userID := c.GetString("userID")
	claimsInterface := c.MustGet("claims")
	claims, ok := claimsInterface.(jwt.MapClaims)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid claims format"})
		return
	}
	ownerID, ok := claims["ownerId"].(string)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid ownerId in claims"})
		return
	}

	// Read YAML content from request body
	yamlContent, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
		return
	}

	statusCode, response := h.contractService.UpdateContract(c.Request.Context(), contractID, ownerID, userID, string(yamlContent))

	// Record metrics
	if statusCode == 200 {
		middleware.RecordContractValidation(true)
		middleware.RecordContractVersionCreated()
	} else {
		middleware.RecordContractValidation(false)
	}

	c.JSON(statusCode, response)
}

func (h *ContractHandler) DeleteContract(c *gin.Context) {
	contractID := c.Param("id")
	claimsInterface := c.MustGet("claims")
	claims, ok := claimsInterface.(jwt.MapClaims)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid claims format"})
		return
	}
	ownerID, ok := claims["ownerId"].(string)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid ownerId in claims"})
		return
	}

	statusCode, response := h.contractService.DeleteContract(c.Request.Context(), contractID, ownerID)
	c.JSON(statusCode, response)
}

func (h *ContractHandler) GetAllContractVersions(c *gin.Context) {
	contractID := c.Param("id")
	claimsInterface := c.MustGet("claims")
	claims, ok := claimsInterface.(jwt.MapClaims)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid claims format"})
		return
	}
	requesterID, ok := claims["ownerId"].(string)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid ownerId in claims"})
		return
	}

	statusCode, response := h.contractService.GetAllContractVersions(c.Request.Context(), contractID, requesterID)
	c.JSON(statusCode, response)
}

func (h *ContractHandler) DeleteContractVersion(c *gin.Context) {
	contractID := c.Param("id")
	versionParam := c.Param("version")

	claimsInterface := c.MustGet("claims")
	claims, ok := claimsInterface.(jwt.MapClaims)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid claims format"})
		return
	}
	ownerID, ok := claims["ownerId"].(string)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid ownerId in claims"})
		return
	}

	// Parse version parameter
	version, err := strconv.Atoi(versionParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Valid version number is required"})
		return
	}

	statusCode, response := h.contractService.DeleteContractVersion(c.Request.Context(), contractID, version, ownerID)
	c.JSON(statusCode, response)
}
