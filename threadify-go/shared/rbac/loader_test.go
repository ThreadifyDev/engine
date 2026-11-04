package rbac

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoaderResolvesAPILevelServiceAccountRoles(t *testing.T) {
	config := fstest.MapFS{
		"permissions.json": {Data: []byte(`{"app_level":[],"runtime_level":[]}`)},
		"roles.json": {Data: []byte(`{
			"app_level": {"member": {"permissions":["entity_profile_type.read"]}},
			"api_level": {"standard_service": {"permissions":["entity_profile_type.update"]}},
			"runtime_level": {"owner": {"permissions":["thread.write.*"]}}
		}`)},
	}
	loader, err := NewLoaderFromFS(config, "permissions.json", "roles.json")
	require.NoError(t, err)

	assert.Equal(t, []string{"entity_profile_type.update"}, loader.GetPermissionsForRoles([]string{"standard_service"}, "api_level"))
	assert.Empty(t, loader.GetPermissionsForRoles([]string{"standard_service"}, "app_level"))
}

func TestEmbeddedObserverIsReadOnly(t *testing.T) {
	loader, err := NewEmbeddedLoader()
	require.NoError(t, err)

	observer := loader.GetPermissionsForRoles([]string{"observer"}, "runtime_level")
	assert.True(t, loader.CheckPermission(observer, "thread.read.*"))
	assert.False(t, loader.CheckPermission(observer, "thread.write.*"), "observers must not report steps")
	assert.False(t, loader.CheckPermission(observer, "thread.end"))

	participant := loader.GetPermissionsForRoles([]string{"participant"}, "runtime_level")
	assert.True(t, loader.CheckPermission(participant, "thread.write.*"))
}
