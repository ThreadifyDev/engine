package api

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEntityProfileTypes_Lifecycle(t *testing.T) {
	user := setupAuthenticatedUser(t)

	t.Run("validation_error_blank_name", func(t *testing.T) {
		resp := doJSONWithAuth(t, http.MethodPost, "/api/entity-profile-types", map[string]any{
			"name": " ",
		}, user.AccessToken)
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)

		body := decodeJSONBody(t, resp)
		assert.NotEmpty(t, body["error"], "validation failure must carry an error message")
	})

	t.Run("validation_error_missing_required_fields", func(t *testing.T) {
		resp := doJSONWithAuth(t, http.MethodPost, "/api/entity-profile-types", map[string]any{}, user.AccessToken)
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)

		body := decodeJSONBody(t, resp)
		assert.NotEmpty(t, body["error"], "missing fields must produce an error message")
	})

	// Create
	createResp := doJSONWithAuth(t, http.MethodPost, "/api/entity-profile-types", map[string]any{
		"name":        "Customer",
		"type":        []string{"customer", "buyer"},
		"description": "A customer entity",
	}, user.AccessToken)
	require.Equal(t, http.StatusCreated, createResp.StatusCode, string(createResp.Body))

	createBody := decodeJSONBody(t, createResp)
	assert.Equal(t, "Entity profile type created successfully.", createBody["message"])

	data, ok := createBody["data"].(map[string]any)
	require.True(t, ok, "create response must include a data object")

	id, ok := data["id"].(string)
	require.True(t, ok, "data must include a string id")
	require.NotEmpty(t, id)

	assert.Equal(t, "Customer", data["name"])
	assert.Equal(t, "customer", data["slug"], "slug should be normalized from name")
	types := data["type"].([]any)
	assert.ElementsMatch(t, []any{"buyer", "customer"}, types)
	assert.Equal(t, "A customer entity", data["description"])
	assert.NotEmpty(t, data["created_at"], "created entity must have a created_at timestamp")

	// List
	listResp := doRawWithAuth(t, http.MethodGet, "/api/entity-profile-types", nil, "", user.AccessToken)
	require.Equal(t, http.StatusOK, listResp.StatusCode)

	listBody := decodeJSONBody(t, listResp)
	assert.Equal(t, "Entity profile types fetched successfully.", listBody["message"])

	items, ok := listBody["data"].([]any)
	require.True(t, ok, "list response data must be an array")
	require.Len(t, items, 1, "expected exactly one entity profile type after create")

	listed, ok := items[0].(map[string]any)
	require.True(t, ok, "list item must be an object")
	assert.Equal(t, id, listed["id"], "listed item must match created id")
	assert.Equal(t, "Customer", listed["name"])
	assert.Equal(t, "customer", listed["slug"], "slug should be normalized from name")
	listedTypes := listed["type"].([]any)
	assert.ElementsMatch(t, []any{"buyer", "customer"}, listedTypes)

	// Update
	updateResp := doJSONWithAuth(t, http.MethodPut, "/api/entity-profile-types/"+id, map[string]any{
		"name":        "Customer Updated",
		"description": "Updated description",
		"type":        []string{"customer", "buyer", "vip"},
	}, user.AccessToken)
	require.Equal(t, http.StatusOK, updateResp.StatusCode, string(updateResp.Body))

	updateBody := decodeJSONBody(t, updateResp)
	assert.Equal(t, "Entity profile type updated successfully.", updateBody["message"])

	updated, ok := updateBody["data"].(map[string]any)
	require.True(t, ok, "update response must include a data object")
	assert.Equal(t, id, updated["id"], "updated item must retain the same id")
	assert.Equal(t, "Customer Updated", updated["name"])
	assert.Equal(t, "customer_updated", updated["slug"], "slug should be normalized from updated name")
	assert.Equal(t, "Updated description", updated["description"])
	updatedTypes := updated["type"].([]any)
	assert.ElementsMatch(t, []any{"customer", "buyer", "vip"}, updatedTypes)

	// Verify list reflects the update
	listAfterUpdate := doRawWithAuth(t, http.MethodGet, "/api/entity-profile-types", nil, "", user.AccessToken)
	require.Equal(t, http.StatusOK, listAfterUpdate.StatusCode)

	listAfterUpdateBody := decodeJSONBody(t, listAfterUpdate)
	itemsAfterUpdate, ok := listAfterUpdateBody["data"].([]any)
	require.True(t, ok, "list response data must be an array after update")
	require.Len(t, itemsAfterUpdate, 1)

	listedAfterUpdate := itemsAfterUpdate[0].(map[string]any)
	assert.Equal(t, "Customer Updated", listedAfterUpdate["name"], "list must reflect updated name")
	assert.Equal(t, "customer_updated", listedAfterUpdate["slug"], "list must reflect updated slug")
	updatedListedTypes := listedAfterUpdate["type"].([]any)
	assert.ElementsMatch(t, []any{"customer", "buyer", "vip"}, updatedListedTypes)

	// Archive
	archiveResp := doRawWithAuth(t, http.MethodDelete, "/api/entity-profile-types/"+id, nil, "", user.AccessToken)
	require.Equal(t, http.StatusOK, archiveResp.StatusCode, string(archiveResp.Body))

	archiveBody := decodeJSONBody(t, archiveResp)
	assert.Equal(t, "Entity profile type archived safely.", archiveBody["message"])

	// Verify list is now empty after archive
	listAfterArchive := doRawWithAuth(t, http.MethodGet, "/api/entity-profile-types", nil, "", user.AccessToken)
	require.Equal(t, http.StatusOK, listAfterArchive.StatusCode)

	listAfterArchiveBody := decodeJSONBody(t, listAfterArchive)
	assertEmptyOrNilList(t, listAfterArchiveBody, "data")
}

func TestEntityProfileTypes_Unauthorized(t *testing.T) {
	endpoints := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/entity-profile-types"},
		{http.MethodPost, "/api/entity-profile-types"},
		{http.MethodPut, "/api/entity-profile-types/some-id"},
		{http.MethodDelete, "/api/entity-profile-types/some-id"},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+"_"+ep.path, func(t *testing.T) {
			resp := doRaw(t, ep.method, ep.path, nil, "")
			require.Equal(t, http.StatusUnauthorized, resp.StatusCode,
				"expected 401 for unauthenticated %s %s, got %d: %s",
				ep.method, ep.path, resp.StatusCode, string(resp.Body))

			body := decodeJSONBody(t, resp)
			assert.NotEmpty(t, body["error"], "unauthorized response must carry an error message")
		})
	}
}

func TestEntityProfileTypes_UpdateValidation(t *testing.T) {
	user := setupAuthenticatedUser(t)

	// First create a profile type to update
	createResp := doJSONWithAuth(t, http.MethodPost, "/api/entity-profile-types", map[string]any{
		"name":        "Test Profile",
		"type":        []string{"customer", "buyer"},
		"description": "Test description",
	}, user.AccessToken)
	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	createBody := decodeJSONBody(t, createResp)
	data := createBody["data"].(map[string]any)
	id := data["id"].(string)

	t.Run("update_exceeds_max_types", func(t *testing.T) {
		resp := doJSONWithAuth(t, http.MethodPut, "/api/entity-profile-types/"+id, map[string]any{
			"name":        "Updated Profile",
			"type":        []string{"type1", "type2", "type3", "type4", "type5", "type6"},
			"description": "Updated description",
		}, user.AccessToken)
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)

		body := decodeJSONBody(t, resp)
		assert.NotEmpty(t, body["error"])
		assert.Contains(t, body["error"], "Validation failed")
	})

	t.Run("update_exactly_max_types", func(t *testing.T) {
		resp := doJSONWithAuth(t, http.MethodPut, "/api/entity-profile-types/"+id, map[string]any{
			"name":        "Max Types Profile",
			"type":        []string{"type1", "type2", "type3", "type4", "type5"},
			"description": "Profile with exactly max types",
		}, user.AccessToken)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		body := decodeJSONBody(t, resp)
		assert.Equal(t, "Entity profile type updated successfully.", body["message"])
	})

	t.Run("update_with_duplicate_types", func(t *testing.T) {
		resp := doJSONWithAuth(t, http.MethodPut, "/api/entity-profile-types/"+id, map[string]any{
			"name":        "Duplicate Types Profile",
			"type":        []string{"customer", "vip", "customer", "partner"},
			"description": "Profile with duplicate types",
		}, user.AccessToken)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		body := decodeJSONBody(t, resp)
		assert.Equal(t, "Entity profile type updated successfully.", body["message"])

		// Verify duplicates are normalized
		updated := body["data"].(map[string]any)
		types := updated["type"].([]any)
		// Should contain unique types only
		uniqueTypes := make(map[string]bool)
		for _, t := range types {
			uniqueTypes[t.(string)] = true
		}
		assert.Len(t, uniqueTypes, len(types), "Types should be deduplicated")
	})

	t.Run("update_with_empty_and_whitespace_types", func(t *testing.T) {
		resp := doJSONWithAuth(t, http.MethodPut, "/api/entity-profile-types/"+id, map[string]any{
			"name":        "Whitespace Types Profile",
			"type":        []string{"vip", "", "  ", "\tpartner\t"},
			"description": "Profile with whitespace types",
		}, user.AccessToken)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		body := decodeJSONBody(t, resp)
		assert.Equal(t, "Entity profile type updated successfully.", body["message"])

		// Verify whitespace is filtered
		updated := body["data"].(map[string]any)
		types := updated["type"].([]any)
		assert.NotContains(t, types, "", "Empty strings should be filtered")
		assert.NotContains(t, types, "  ", "Whitespace should be filtered")
	})
}

func TestEntityProfileTypes_SlugNormalization(t *testing.T) {
	user := setupAuthenticatedUser(t)

	t.Run("slug_normalization_with_special_characters", func(t *testing.T) {
		resp := doJSONWithAuth(t, http.MethodPost, "/api/entity-profile-types", map[string]any{
			"name":        "Customer Support Agent!!",
			"type":        []string{"customer", "support"},
			"description": "A customer support agent profile",
		}, user.AccessToken)
		require.Equal(t, http.StatusCreated, resp.StatusCode)

		body := decodeJSONBody(t, resp)
		data := body["data"].(map[string]any)
		assert.Equal(t, "Customer Support Agent!!", data["name"])
		assert.Equal(t, "customer_support_agent", data["slug"], "special characters should be normalized to underscores")
	})

	t.Run("slug_normalization_with_multiple_spaces_and_special_chars", func(t *testing.T) {
		resp := doJSONWithAuth(t, http.MethodPost, "/api/entity-profile-types", map[string]any{
			"name":        "VIP   Customer++Manager",
			"type":        []string{"vip", "customer", "manager"},
			"description": "A VIP customer manager profile",
		}, user.AccessToken)
		require.Equal(t, http.StatusCreated, resp.StatusCode)

		body := decodeJSONBody(t, resp)
		data := body["data"].(map[string]any)
		assert.Equal(t, "VIP   Customer++Manager", data["name"])
		assert.Equal(t, "vip_customer_manager", data["slug"], "multiple spaces and special chars should be normalized")
	})
}

func TestEntityProfileTypes_CreateValidation(t *testing.T) {
	user := setupAuthenticatedUser(t)

	t.Run("create_exceeds_max_types", func(t *testing.T) {
		resp := doJSONWithAuth(t, http.MethodPost, "/api/entity-profile-types", map[string]any{
			"name":        "Too Many Types",
			"type":        []string{"type1", "type2", "type3", "type4", "type5", "type6"},
			"description": "Profile with too many types",
		}, user.AccessToken)
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)

		body := decodeJSONBody(t, resp)
		assert.NotEmpty(t, body["error"])
		assert.Contains(t, body["error"], "Validation failed")
	})

	t.Run("create_no_valid_types", func(t *testing.T) {
		resp := doJSONWithAuth(t, http.MethodPost, "/api/entity-profile-types", map[string]any{
			"name":        "No Valid Types",
			"type":        []string{"", "  ", "\t"},
			"description": "Profile with no valid types",
		}, user.AccessToken)
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)

		body := decodeJSONBody(t, resp)
		assert.NotEmpty(t, body["error"])
		assert.Contains(t, body["error"], "Validation failed")
	})

	t.Run("create_with_duplicate_types", func(t *testing.T) {
		resp := doJSONWithAuth(t, http.MethodPost, "/api/entity-profile-types", map[string]any{
			"name":        "Duplicate Types",
			"type":        []string{"customer", "vip", "customer", "partner"},
			"description": "Profile with duplicate types",
		}, user.AccessToken)
		require.Equal(t, http.StatusCreated, resp.StatusCode)

		body := decodeJSONBody(t, resp)
		assert.Equal(t, "Entity profile type created successfully.", body["message"])

		// Verify duplicates are normalized
		created := body["data"].(map[string]any)
		types := created["type"].([]any)
		// Should contain unique types only
		uniqueTypes := make(map[string]bool)
		for _, t := range types {
			uniqueTypes[t.(string)] = true
		}
		assert.Len(t, uniqueTypes, len(types), "Types should be deduplicated")
	})
}

func TestEntityProfile_GetEntityProfile_MissingParams(t *testing.T) {
	user := setupAuthenticatedUser(t)

	t.Run("missing_both", func(t *testing.T) {
		resp := doRawWithAuth(t, http.MethodGet, "/api/entity-profiles", nil, "", user.AccessToken)
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)

		body := decodeJSONBody(t, resp)
		assert.Equal(t, "refKey and type query params are required", body["error"])
	})

	t.Run("missing_type", func(t *testing.T) {
		resp := doRawWithAuth(t, http.MethodGet, "/api/entity-profiles?refKey=abc", nil, "", user.AccessToken)
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)

		body := decodeJSONBody(t, resp)
		assert.Equal(t, "refKey and type query params are required", body["error"])
	})

	t.Run("missing_refKey", func(t *testing.T) {
		resp := doRawWithAuth(t, http.MethodGet, "/api/entity-profiles?type=customer", nil, "", user.AccessToken)
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)

		body := decodeJSONBody(t, resp)
		assert.Equal(t, "refKey and type query params are required", body["error"])
	})
}

func TestEntityProfile_EngineUnavailable(t *testing.T) {
	user := setupAuthenticatedUser(t)

	resp := doRawWithAuth(t, http.MethodGet, "/api/entity-profiles?refKey=abc&type=customer", nil, "", user.AccessToken)
	require.Equal(t, http.StatusBadGateway, resp.StatusCode,
		"expected 502 when engine is not running: %s", string(resp.Body))

	body := decodeJSONBody(t, resp)
	assert.NotEmpty(t, body["error"])
}
