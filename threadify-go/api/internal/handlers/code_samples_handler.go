package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
)

type CodeSamplesHandler struct {
	samplesPath string
}

func NewCodeSamplesHandler(samplesPath string) *CodeSamplesHandler {
	return &CodeSamplesHandler{
		samplesPath: samplesPath,
	}
}

// GetCodeSample returns code samples for different languages and types
func (h *CodeSamplesHandler) GetCodeSample(c *gin.Context) {
	codeType := c.DefaultQuery("codeType", "basic_instrumentation")

	// Read the code samples JSON file
	filePath := filepath.Join(h.samplesPath, "code_samples.json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to load code samples",
		})
		return
	}

	var samples map[string]map[string]string
	if err := json.Unmarshal(data, &samples); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to parse code samples",
		})
		return
	}

	// Get samples for the requested type
	typeSamples, exists := samples[codeType]
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Code type not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code_type": codeType,
		"samples":   typeSamples,
	})
}
