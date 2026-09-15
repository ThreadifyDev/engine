package rbac

import "embed"

//go:embed permissions.json roles.json
var embeddedConfig embed.FS

// NewEmbeddedLoader loads the built-in permissions and roles without requiring
// a source checkout or runtime asset directory.
func NewEmbeddedLoader() (*Loader, error) {
	return NewLoaderFromFS(embeddedConfig, "permissions.json", "roles.json")
}
