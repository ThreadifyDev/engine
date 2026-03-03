package tests

import "github.com/threadify/engine/internal/service"

type InvitationConfig = service.InvitationConfig
type InvitationTokenService = service.InvitationTokenService

func NewInvitationTokenService(secretKey string) *InvitationTokenService {
	return service.NewInvitationTokenService(secretKey, "test-issuer")
}
