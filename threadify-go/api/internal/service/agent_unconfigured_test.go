package service

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
	"threadify-go/shared/management/domain"
)

func TestChatStreamEino_UnconfiguredModelDoesNotCreateConversation(t *testing.T) {
	// Nil repositories deliberately ensure the configuration failure occurs before
	// any conversation is persisted or any model/network work is attempted.
	svc := &AgentService{openaiAPIKey: "  "}
	var events []string
	err := svc.ChatStreamEino(context.Background(), "Bearer test", "user", "company", "", "hello", "auto", func(kind, message string) {
		require.Equal(t, domain.EventError, kind)
		events = append(events, message)
	})
	require.Error(t, err)
	require.Len(t, events, 1)
	require.Contains(t, events[0], "no model provider is configured")
}
