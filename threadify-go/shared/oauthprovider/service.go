package oauthprovider

import "github.com/Usefused/fused-open-core/oauthserver"

// Threadify owns the delegated scope allowlist. The OAuth protocol stays in fused-open-core.
const (
	ScopeContractReadAll = "contract.read.*"
	ScopeMCPRead         = "query.execution.read"
)

func PilotScopes() []string { return []string{ScopeContractReadAll, ScopeMCPRead} }

var Prefixes = oauthserver.Prefixes{
	ClientID: "toc_", ClientSecret: "tos_", AccessToken: "toat_", RefreshToken: "tort_",
}

// NewService binds the shared OAuth protocol to Threadify's string user IDs,
// role policy, token namespace, and product-owned durable store.
func NewService(store oauthserver.Store[string], revisions oauthserver.RevisionSink, policy *Policy) (*oauthserver.Service[string], error) {
	return oauthserver.NewService[string](store, revisions, policy, Prefixes)
}
