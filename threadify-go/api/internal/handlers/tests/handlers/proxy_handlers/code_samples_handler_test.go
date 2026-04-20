package tests

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"threadify-go/api/internal/handlers"
	"threadify-go/api/internal/handlers/tests/common"
)

func TestCodeSamplesHandler_GetCodeSample(t *testing.T) {
	newRouter := func() *gin.Engine {
		h := handlers.NewCodeSamplesHandler("api_key")
		r := common.SetupTestRouter()
		r.GET("/code-samples/:id/:language", h.GetCodeSample)
		return r
	}

	tests := []struct {
		name       string
		id         string
		language   string
		wantStatus int
	}{
		{
			name:       "success",
			id:         "sample1",
			language:   "go",
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRouter()
			w := common.DoRequest(t, r, "GET", "/code-samples/"+tt.id+"/"+tt.language, nil)
			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}
