package handlers

import (
	"fmt"
	"io"
	"net/http"
	"strconv"

	"go.uber.org/zap"

	sharedauth "threadify-go/shared/auth"

	"github.com/gin-gonic/gin"
	"github.com/threadify/engine/internal/types"
	"github.com/threadify/engine/internal/middleware"
)

type PreviewResponse struct {
	Valid    bool        `json:"valid"`
	Errors   []string    `json:"errors,omitempty"`
	Graph    interface{} `json:"graph,omitempty"`
	Contract interface{} `json:"contract,omitempty"`
}

type ContractHandler struct {
	contractService types.ContractService
	logger          *zap.Logger
}

func NewContractHandler(contractService types.ContractService, logger *zap.Logger) *ContractHandler {
	return &ContractHandler{contractService: contractService, logger: logger}
}

// claimsOwnerID extracts ownerID from the gin context (set by AuthMiddleware).
// Returns ("", false) and writes a JSON error if extraction fails.
func claimsOwnerID(c *gin.Context) (string, bool) {
	ownerID, exists := c.Get(sharedauth.CtxUserID)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return "", false
	}
	ownerIDStr, ok := ownerID.(string)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid userID format"})
		return "", false
	}
	return ownerIDStr, true
}

// companyIDFromContext resolves company ID from authenticated context.
func companyIDFromContext(c *gin.Context) (string, error) {
	var contextID string
	if companyID, exists := c.Get(sharedauth.CtxCompanyID); exists {
		if id, ok := companyID.(string); ok {
			contextID = id
		}
	}

	if contextID == "" {
		return "", fmt.Errorf("company id missing from authentication context")
	}

	return contextID, nil
}

// recordContractMetrics records Prometheus metrics based on the service response code.
func recordContractMetrics(statusCode int) {
	if statusCode == http.StatusOK {
		middleware.RecordContractValidation(true)
		middleware.RecordContractVersionCreated()
	} else {
		middleware.RecordContractValidation(false)
	}
}

func (h *ContractHandler) GetAllContracts(c *gin.Context) {
	ownerID, ok := claimsOwnerID(c)
	if !ok {
		return
	}

	search := c.Query("search")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "0"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	statusCode, response := h.contractService.GetAllContracts(c.Request.Context(), ownerID, search, limit, offset)
	c.JSON(statusCode, response)
}

func (h *ContractHandler) CreateContract(c *gin.Context) {
	ownerID, ok := claimsOwnerID(c)
	if !ok {
		return
	}

	companyID, err := companyIDFromContext(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	yamlBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
		return
	}

	statusCode, response := h.contractService.CreateContract(
		c.Request.Context(), ownerID, companyID,
		c.GetString(sharedauth.CtxUserID), string(yamlBytes),
	)
	recordContractMetrics(statusCode)
	c.JSON(statusCode, response)
}

func (h *ContractHandler) GetContract(c *gin.Context) {
	ownerID, ok := claimsOwnerID(c)
	if !ok {
		return
	}

	var version *int
	if v := c.Query("version"); v != "" {
		if ver, err := strconv.Atoi(v); err == nil {
			version = &ver
		}
	}

	statusCode, response := h.contractService.GetContract(c.Request.Context(), c.Param("id"), ownerID, version)
	c.JSON(statusCode, response)
}

func (h *ContractHandler) UpdateContract(c *gin.Context) {
	ownerID, ok := claimsOwnerID(c)
	if !ok {
		return
	}

	yamlContent, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
		return
	}

	statusCode, response := h.contractService.UpdateContract(
		c.Request.Context(), c.Param("id"), ownerID,
		c.GetString(sharedauth.CtxUserID), string(yamlContent),
	)
	recordContractMetrics(statusCode)
	c.JSON(statusCode, response)
}

func (h *ContractHandler) DeleteContract(c *gin.Context) {
	ownerID, ok := claimsOwnerID(c)
	if !ok {
		return
	}
	statusCode, response := h.contractService.DeleteContract(c.Request.Context(), c.Param("id"), ownerID)
	c.JSON(statusCode, response)
}

func (h *ContractHandler) GetAllContractVersions(c *gin.Context) {
	ownerID, ok := claimsOwnerID(c)
	if !ok {
		return
	}
	statusCode, response := h.contractService.GetAllContractVersions(c.Request.Context(), c.Param("id"), ownerID)
	c.JSON(statusCode, response)
}

func (h *ContractHandler) GetContractVersion(c *gin.Context) {
	ownerID, ok := claimsOwnerID(c)
	if !ok {
		return
	}

	version, err := strconv.Atoi(c.Param("version"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Valid version number is required"})
		return
	}

	statusCode, response := h.contractService.GetContractVersion(c.Request.Context(), c.Param("id"), version, ownerID)
	c.JSON(statusCode, response)
}

func (h *ContractHandler) DeleteContractVersion(c *gin.Context) {
	ownerID, ok := claimsOwnerID(c)
	if !ok {
		return
	}

	version, err := strconv.Atoi(c.Param("version"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Valid version number is required"})
		return
	}

	statusCode, response := h.contractService.DeleteContractVersion(c.Request.Context(), c.Param("id"), version, ownerID)
	c.JSON(statusCode, response)
}

func (h *ContractHandler) PreviewContract(c *gin.Context) {
	yamlBody, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, PreviewResponse{Valid: false, Errors: []string{"Failed to read request body"}})
		return
	}

	contract, graph, validationResult, err := h.contractService.PreviewContract(string(yamlBody))
	if err != nil {
		h.logger.Error("failed to preview contract", zap.Error(err))
		c.JSON(http.StatusInternalServerError, PreviewResponse{Valid: false, Errors: []string{"Failed to process contract"}})
		return
	}

	if !validationResult.IsValid {
		errs := make([]string, len(validationResult.Errors))
		for i, e := range validationResult.Errors {
			errs[i] = fmt.Sprintf("%s: %s", e.Field, e.Message)
		}
		c.JSON(http.StatusOK, PreviewResponse{Valid: false, Errors: errs})
		return
	}

	c.JSON(http.StatusOK, PreviewResponse{
		Valid:    true,
		Graph:    graph,
		Contract: contract,
	})
}
