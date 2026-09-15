package service

import (
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/domain"
	"testing"
)

func TestOTelLifecycleOnlyOwnsContractFreeCorrelatedThreads(t *testing.T) {
	traceID := "01020304050607080910111213141516"
	thread := &domain.Thread{ID: uuid.NewSHA1(uuid.NameSpaceOID, []byte("company:"+traceID)).String(), CompanyID: "company", Refs: map[string]string{"otel_trace_id": traceID}}
	require.True(t, isOwnedOTelThread(thread, "company", traceID))
	require.False(t, isOwnedOTelThread(thread, "other", traceID))
	require.False(t, isOwnedOTelThread(thread, "company", ""))
	require.False(t, isOwnedOTelThread(thread, "company", "another-trace"))
	thread.ContractName = "workflow"
	require.False(t, isOwnedOTelThread(thread, "company", traceID))
	thread.ContractName = ""
	contractID := "contract"
	thread.ContractID = &contractID
	require.False(t, isOwnedOTelThread(thread, "company", traceID))
	thread.ContractID = nil
	thread.ID = uuid.NewString()
	require.False(t, isOwnedOTelThread(thread, "company", traceID), "explicit existing workflows must not auto-close")
}
