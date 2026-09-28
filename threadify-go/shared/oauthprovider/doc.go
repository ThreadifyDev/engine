// Package oauthprovider adapts fused-open-core's OAuth authorization server to
// Threadify identities and permissions. The protocol remains in open-core;
// Threadify owns its role source, delegated-scope allowlist, and token store.
// This package is the integration seam used for the first cross-product test.
package oauthprovider
