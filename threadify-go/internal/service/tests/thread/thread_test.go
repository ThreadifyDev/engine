package service_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/models"
	"github.com/threadify/engine/internal/service"
)

func TestParseContractIdentifier(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantName string
		wantVer  int
	}{
		{"no version suffix", "my_contract", "my_contract", 0},
		{"version suffix", "my_contract:3", "my_contract", 3},
		{"zero version ignored", "my_contract:0", "my_contract:0", 0},
		{"negative version ignored", "my_contract:-1", "my_contract:-1", 0},
		{"non-numeric suffix ignored", "my_contract:abc", "my_contract:abc", 0},
		{"multiple colons uses last", "ns:my_contract:5", "ns:my_contract", 5},
		{"empty string", "", "", 0},
		{"colon only", ":", ":", 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			name, ver := service.ParseContractIdentifier(tc.input)
			assert.Equal(t, tc.wantName, name)
			assert.Equal(t, tc.wantVer, ver)
		})
	}
}

func TestValidateRecordEventRequest(t *testing.T) {
	validReq := &models.RecordEventRequest{
		ThreadID:   "t1",
		StepName:   "step_a",
		Status:     "success",
		StartedAt:  time.Now().Format(time.RFC3339),
		FinishedAt: time.Now().Format(time.RFC3339),
		Context:    map[string]string{"k": "v"},
	}

	tests := []struct {
		name    string
		mutate  func(*models.RecordEventRequest)
		wantErr string
	}{
		{"valid", func(r *models.RecordEventRequest) {}, ""},
		{"missing thread id", func(r *models.RecordEventRequest) { r.ThreadID = "" }, "Thread ID is required"},
		{"missing step name", func(r *models.RecordEventRequest) { r.StepName = "" }, "StepName is required"},
		{"missing status", func(r *models.RecordEventRequest) { r.Status = "" }, "Status is required"},
		{"missing started at", func(r *models.RecordEventRequest) { r.StartedAt = "" }, "StartedAt is required"},
		{"missing finished at", func(r *models.RecordEventRequest) { r.FinishedAt = "" }, "FinishedAt is required"},
		{"missing context", func(r *models.RecordEventRequest) { r.Context = nil }, "Context is required"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := *validReq
			tc.mutate(&req)
			err := service.ValidateRecordEventRequest(&req)
			if tc.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
			}
		})
	}
}
