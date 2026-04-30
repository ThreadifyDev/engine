package handlers

import (
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type PricingHandler struct {
	threadifyEngineURL string
	httpClient         *http.Client
}

func NewPricingHandler(threadifyEngineURL string) *PricingHandler {
	return &PricingHandler{
		threadifyEngineURL: threadifyEngineURL,
		httpClient:         &http.Client{Timeout: 10 * time.Second},
	}
}

func (h *PricingHandler) GetPricing(c *gin.Context) {
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, h.threadifyEngineURL+"/v1/pricing", nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create request"})
		return
	}

	resp, err := h.httpClient.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to connect to ThreadifyEngine"})
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read response"})
		return
	}

	c.Data(resp.StatusCode, "application/json", respBody)
}
