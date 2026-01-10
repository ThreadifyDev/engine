package graphql

import (
	"github.com/threadify/engine/internal/repository/valkey"
)

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require
// here.

type Resolver struct {
	threadRepo     *valkey.ThreadRepository
	stepStateRepo  *valkey.StepStateRepository
	validationRepo *valkey.ValidationRepository
}

func NewResolver(threadRepo *valkey.ThreadRepository, stepStateRepo *valkey.StepStateRepository, validationRepo *valkey.ValidationRepository) *Resolver {
	return &Resolver{
		threadRepo:     threadRepo,
		stepStateRepo:  stepStateRepo,
		validationRepo: validationRepo,
	}
}
