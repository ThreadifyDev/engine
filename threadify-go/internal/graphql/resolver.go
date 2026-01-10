package graphql

import (
	"github.com/threadify/engine/internal/repository/valkey"
)

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require
// here.

type Resolver struct {
	threadRepo *valkey.ThreadRepository
}

func NewResolver(threadRepo *valkey.ThreadRepository) *Resolver {
	return &Resolver{
		threadRepo: threadRepo,
	}
}
