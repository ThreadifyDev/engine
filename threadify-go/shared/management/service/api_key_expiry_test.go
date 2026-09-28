package service

import (
	"context"
	"testing"
	"time"

	"threadify-go/shared/management/domain"

	"go.uber.org/zap"
)

type expiryKeyRepository struct {
	domain.APIKeyRepository
	created *domain.APIKey
}

func (r *expiryKeyRepository) Create(_ context.Context, key *domain.APIKey) error {
	r.created = key
	return nil
}

type expiryServiceAccountRepository struct {
	domain.ServiceAccountRepository
	account *domain.ServiceAccount
}

func (r *expiryServiceAccountRepository) FindByID(_ context.Context, _ string) (*domain.ServiceAccount, error) {
	return r.account, nil
}

func TestCreateAPIKeyStoresCustomExpiry(t *testing.T) {
	accountID := "account-1"
	expiry := time.Now().Add(48 * time.Hour).Truncate(time.Millisecond)
	keys := &expiryKeyRepository{}
	accounts := &expiryServiceAccountRepository{account: &domain.ServiceAccount{ID: accountID, CompanyID: "company-1"}}
	service := NewAPIKeyService(keys, accounts, nil, nil, zap.NewNop())

	_, err := service.CreateAPIKey(context.Background(), "user-1", "company-1", &domain.CreateAPIKeyCmd{
		Name: "test", ServiceAccountID: &accountID, ExpiresAt: &expiry,
	})
	if err != nil {
		t.Fatalf("create API key: %v", err)
	}
	if keys.created == nil || keys.created.ExpiresAt == nil || !keys.created.ExpiresAt.Equal(expiry) {
		t.Fatalf("custom expiry was not stored: %+v", keys.created)
	}
}

func TestCreateAPIKeyRejectsPastExpiryBeforeCreatingAccount(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	service := NewAPIKeyService(nil, nil, nil, nil, zap.NewNop())
	_, err := service.CreateAPIKey(context.Background(), "user-1", "company-1", &domain.CreateAPIKeyCmd{
		Name: "test", CreateServiceAccount: true, ExpiresAt: &past,
	})
	if err != ErrInvalidApiKeyExpiry {
		t.Fatalf("expected expiry error before creating account, got %v", err)
	}
}
