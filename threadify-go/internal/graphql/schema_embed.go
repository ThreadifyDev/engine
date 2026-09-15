package graphql

import _ "embed"

// Schema is the GraphQL schema bundled with this engine release.
//
//go:embed schema.graphql
var Schema []byte
