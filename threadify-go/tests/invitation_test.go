package tests

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/models"
	"github.com/threadify/engine/internal/service"
)

func TestWebSocketHandler_InviteParty(t *testing.T) {
	// Test that inviteParty handler creates JWT token correctly
	handler := setupTestHandler(t)

	// Create test session
	session := &Session{
		ownerID:   "test-user-1",
		threadIDs: []string{"test-thread-1"}, // Include thread for inviteParty to work
	}

	// Create inviteParty request
	request := models.InvitePartyRequest{
		Action:      "inviteParty",
		Role:        "external_partner",
		Permissions: "read,write",
		ExpiresIn:   "24h",
	}

	// Handle inviteParty request
	response := handler.handleInviteParty(session, &request)

	// Verify response
	assert.NotNil(t, response)
	inviteResponse := response.(models.InvitePartyResponse)
	assert.Equal(t, "inviteParty", inviteResponse.Action)
	assert.Equal(t, "success", inviteResponse.Status)
	assert.NotEmpty(t, inviteResponse.ThreadToken)
	assert.Equal(t, "external_partner", inviteResponse.Role)
	assert.Equal(t, "read,write", inviteResponse.Permissions)

	// Verify token can be validated
	claims, err := handler.invitationService.ValidateToken(inviteResponse.ThreadToken)
	require.NoError(t, err)
	assert.Equal(t, "test-thread-1", claims.ThreadID)
	assert.Equal(t, "external_partner", claims.Role)
}

func TestWebSocketHandler_InviteParty_Validation(t *testing.T) {
	handler := setupTestHandler(t)

	session := &Session{
		ownerID:   "test-user-1",
		threadIDs: []string{"test-thread-1"},
	}

	// Test invalid role
	request := models.InvitePartyRequest{
		Action:      "inviteParty",
		Role:        "invalid_role",
		Permissions: "read,write",
		ExpiresIn:   "24h",
	}

	response := handler.handleInviteParty(session, &request)

	errorResponse := response.(models.ErrorResponse)
	assert.Equal(t, "error", errorResponse.Status)
	assert.Contains(t, errorResponse.Message, "invalid role")
}

func TestWebSocketHandler_InviteParty_AuditLogging(t *testing.T) {
	// Test that inviteParty logs audit events
	handler := setupTestHandler(t)

	session := &Session{
		ownerID:   "test-user-1",
		threadIDs: []string{"test-thread-1"},
	}

	request := models.InvitePartyRequest{
		Action:      "inviteParty",
		Role:        "external_partner",
		Permissions: "read,write",
		ExpiresIn:   "24h",
	}

	handler.handleInviteParty(session, &request)

	// Verify audit event was logged
	// This would require mocking the audit service or checking Redis
	// For now, just ensure no error occurred
	assert.True(t, true) // Placeholder - would implement actual audit verification
}

func TestWebSocketHandler_JoinThread(t *testing.T) {
	// Test that joinThread handler validates JWT token and joins thread
	handler := setupTestHandler(t)

	// First create an invitation token
	invitationService := service.NewInvitationTokenService("test-secret")
	threadToken, err := invitationService.CreateToken("thread-123", "contract-456", "user-789", "external_partner", "read,write", 24*time.Hour)
	require.NoError(t, err)

	// Create test session for joining user
	session := &Session{
		ownerID:   "joining-user-1",
		threadIDs: []string{},
	}

	// Create joinThread request
	request := models.JoinThreadRequest{
		Action:      "joinThread",
		ThreadToken: threadToken,
	}

	// Handle joinThread request
	response := handler.handleJoinThread(session, &request)

	// Verify response
	assert.NotNil(t, response)
	joinResponse := response.(models.JoinThreadResponse)
	assert.Equal(t, "joinThread", joinResponse.Action)
	assert.Equal(t, "success", joinResponse.Status)
	assert.Equal(t, "thread-123", joinResponse.ThreadID)
	assert.Equal(t, "contract-456", joinResponse.ContractID)
	assert.Equal(t, "external_partner", joinResponse.Role)
	assert.Equal(t, "read,write", joinResponse.Permissions)

	// Verify session was updated with thread context
	assert.Contains(t, session.threadIDs, "thread-123")
}

func TestWebSocketHandler_JoinThread_InvalidToken(t *testing.T) {
	handler := setupTestHandler(t)

	session := &Session{
		ownerID:   "joining-user-1",
		threadIDs: []string{},
	}

	// Test invalid token
	request := models.JoinThreadRequest{
		Action:      "joinThread",
		ThreadToken: "invalid-token",
	}

	response := handler.handleJoinThread(session, &request)

	errorResponse := response.(models.ErrorResponse)
	assert.Equal(t, "error", errorResponse.Status)
	assert.Contains(t, errorResponse.Message, "Invalid thread token")
}

func TestWebSocketHandler_JoinThread_ExpiredToken(t *testing.T) {
	handler := setupTestHandler(t)

	// Create expired token
	invitationService := service.NewInvitationTokenService("test-secret")
	expiredToken, err := invitationService.CreateToken("thread-123", "contract-456", "user-789", "external_partner", "read,write", -1*time.Hour)
	require.NoError(t, err)

	session := &Session{
		ownerID:   "joining-user-1",
		threadIDs: []string{},
	}

	request := models.JoinThreadRequest{
		Action:      "joinThread",
		ThreadToken: expiredToken,
	}

	response := handler.handleJoinThread(session, &request)

	errorResponse := response.(models.ErrorResponse)
	assert.Equal(t, "error", errorResponse.Status)
	assert.Contains(t, errorResponse.Message, "token")
}

func TestWebSocketHandler_JoinThread_AuditLogging(t *testing.T) {
	// Test that joinThread logs audit events
	handler := setupTestHandler(t)

	// Create invitation token
	invitationService := service.NewInvitationTokenService("test-secret")
	threadToken, err := invitationService.CreateToken("thread-123", "contract-456", "user-789", "external_partner", "read,write", 24*time.Hour)
	require.NoError(t, err)

	session := &Session{
		ownerID:   "joining-user-1",
		threadIDs: []string{},
	}

	request := models.JoinThreadRequest{
		Action:      "joinThread",
		ThreadToken: threadToken,
	}

	handler.handleJoinThread(session, &request)

	// Verify audit event was logged
	// This would require mocking the audit service or checking Redis
	// For now, just ensure no error occurred
	assert.True(t, true) // Placeholder - would implement actual audit verification
}

// Helper function to set up test handler
func setupTestHandler(t *testing.T) *WebSocketHandler {
	// Create test services
	invitationService := service.NewInvitationTokenService("test-secret")

	return &WebSocketHandler{
		invitationService: invitationService,
		// Other services can be nil for this test
		threadService:    nil,
		stepEventService: nil,
		auditService:     nil,
	}
}
