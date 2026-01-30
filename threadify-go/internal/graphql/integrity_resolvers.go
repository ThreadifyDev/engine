package graphql

import (
	"context"
	"fmt"

	"github.com/threadify/engine/internal/models"
)

// VerifyThreadIntegrity is the resolver for the verifyThreadIntegrity query
// Verifies the hash chain integrity of a thread (separate from thread query for performance)
func (r *queryResolver) VerifyThreadIntegrity(ctx context.Context, threadID string) (*models.HashChainStatus, error) {
	// Call the activity repository to verify the chain
	status, err := r.activityRepo.VerifyActivityChain(ctx, threadID)
	if err != nil {
		return nil, fmt.Errorf("failed to verify thread integrity: %w", err)
	}
	return status, nil
}

// VerifyStepIntegrity is the resolver for the verifyStepIntegrity query
// Verifies the hash integrity of a single step
func (r *queryResolver) VerifyStepIntegrity(ctx context.Context, threadID string, stepName string, idempotencyKey string) (*models.StepIntegrityStatus, error) {
	// Get the step hashes
	hash, prevHash, err := r.activityRepo.GetStepHashes(ctx, threadID, stepName, idempotencyKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get step hashes: %w", err)
	}

	// Verify the step hash
	verified, errMsg, err := r.activityRepo.VerifyStepHash(ctx, threadID, stepName, idempotencyKey)
	if err != nil {
		return nil, fmt.Errorf("failed to verify step hash: %w", err)
	}

	status := &models.StepIntegrityStatus{
		Verified: verified,
		Hash:     hash,
		PrevHash: prevHash,
	}

	if errMsg != "" {
		status.Error = &errMsg
	}

	return status, nil
}

// Hash is the resolver for the hash field in StepStateInfo
// Returns the cryptographic hash for the step
func (r *stepStateInfoResolver) Hash(ctx context.Context, obj *models.StepStateInfo) (*string, error) {
	hash, _, err := r.activityRepo.GetStepHashes(ctx, obj.ThreadID, obj.StepName, obj.IdempotencyKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get step hash: %w", err)
	}
	if hash == "" {
		return nil, nil
	}
	return &hash, nil
}

// PrevHash is the resolver for the prevHash field in StepStateInfo
// Returns the previous hash in the chain for the step
func (r *stepStateInfoResolver) PrevHash(ctx context.Context, obj *models.StepStateInfo) (*string, error) {
	_, prevHash, err := r.activityRepo.GetStepHashes(ctx, obj.ThreadID, obj.StepName, obj.IdempotencyKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get step prevHash: %w", err)
	}
	if prevHash == "" {
		return nil, nil
	}
	return &prevHash, nil
}

// Hash is the resolver for the hash field in StepHistory
// Returns the cryptographic hash for the step history entry
func (r *stepHistoryResolver) Hash(ctx context.Context, obj *models.StepHistory) (*string, error) {
	if obj.Hash == "" {
		return nil, nil
	}
	return &obj.Hash, nil
}

// PrevHash is the resolver for the prevHash field in StepHistory
// Returns the previous hash in the chain for the step history entry
func (r *stepHistoryResolver) PrevHash(ctx context.Context, obj *models.StepHistory) (*string, error) {
	if obj.PrevHash == "" {
		return nil, nil
	}
	return &obj.PrevHash, nil
}
