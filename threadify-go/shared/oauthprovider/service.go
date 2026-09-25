package oauthprovider

import "github.com/Usefused/fused-open-core/oauthserver"

// Initial delegated access is limited to contract reads. The allowlist is a
// product decision and remains in Threadify, outside fused-open-core.
const ScopeContractReadAll = "contract.read.*"

func PilotScopes() []string { return []string{ScopeContractReadAll} }

var Prefixes = oauthserver.Prefixes{
	ClientID: "toc_", ClientSecret: "tos_", AccessToken: "toat_", RefreshToken: "tort_",
}

// NewService binds the shared OAuth protocol to Threadify's string user IDs,
// role policy, token namespace, and product-owned durable store.
func NewService(store oauthserver.Store[string], revisions oauthserver.RevisionSink, policy *Policy) (*oauthserver.Service[string], error) {
	return oauthserver.NewService[string](store, revisions, policy, Prefixes)
}
