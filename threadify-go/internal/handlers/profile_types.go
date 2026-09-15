package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	sharedauth "threadify-go/shared/auth"
	"threadify-go/shared/domain"
	serror "threadify-go/shared/errors"
	"threadify-go/shared/registry"
	"threadify-go/shared/repository"
	"threadify-go/shared/slug"
)

// CreateProfileType exposes basic profile definitions to Engine-only clients.
// Profiles themselves are materialized from matching thread references.
func CreateProfileType(repo repository.EntityProfileTypeRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		company := c.GetString(sharedauth.CtxCompanyID)
		if company == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "company required"})
			return
		}
		var input struct {
			Name        string   `json:"name"`
			Type        []string `json:"type"`
			Description string   `json:"description"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			c.JSON(400, gin.H{"error": "expected name, type (reference keys), and optional description; metrics configuration is not supported by this endpoint"})
			return
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			c.JSON(400, gin.H{"error": "expected one JSON object"})
			return
		}
		input.Name = strings.TrimSpace(input.Name)
		nameSlug := slug.ToSlug(input.Name)
		if input.Name == "" || len(input.Name) > 255 || nameSlug == "" || len(input.Description) > 255 || len(input.Type) < 1 || len(input.Type) > 5 {
			c.JSON(400, gin.H{"error": "name and 1–5 reference keys are required; name and description must be at most 255 bytes"})
			return
		}
		seen := map[string]bool{}
		for i, key := range input.Type {
			key = strings.TrimSpace(key)
			if key == "" || seen[key] {
				c.JSON(400, gin.H{"error": "reference keys must be nonempty and unique"})
				return
			}
			input.Type[i] = key
			seen[key] = true
		}
		now := time.Now().UTC()
		profileType := &domain.EntityProfileType{ID: uuid.NewString(), CompanyID: company, Name: input.Name, Slug: nameSlug, Type: input.Type, Description: input.Description, CreatedAt: now, UpdatedAt: now}
		if err := repo.CreateProfileType(c.Request.Context(), profileType); err != nil {
			if errors.Is(err, serror.ErrEntityProfileTypeAlreadyExists) {
				c.JSON(409, gin.H{"error": "entity profile type already exists"})
				return
			}
			c.JSON(500, gin.H{"error": "could not create entity profile type"})
			return
		}
		c.JSON(201, gin.H{"data": gin.H{"id": profileType.ID, "company_id": company, "name": profileType.Name, "slug": profileType.Slug, "type": profileType.Type, "description": profileType.Description, "created_at": now, "updated_at": now}})
	}
}

// PutProfile creates or updates a named entity using the same quota-aware
// repository as other ingestion paths. A repeated type/ref pair retains its ID.
func PutProfile(types repository.EntityProfileTypeRepository, profiles repository.EntityProfileRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		company := c.GetString(sharedauth.CtxCompanyID)
		if company == "" {
			c.JSON(401, gin.H{"error": "company required"})
			return
		}
		var input struct {
			TypeID string `json:"type_id"`
			Ref    string `json:"ref_value"`
			Name   string `json:"name"`
		}
		dec := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&input); err != nil {
			c.JSON(400, gin.H{"error": "expected type_id, ref_value and name"})
			return
		}
		if err := dec.Decode(new(any)); err != io.EOF {
			c.JSON(400, gin.H{"error": "expected one JSON object"})
			return
		}
		if _, err := uuid.Parse(input.TypeID); err != nil || strings.TrimSpace(input.Ref) == "" || len(input.Ref) > 255 || strings.TrimSpace(input.Name) == "" || len(input.Name) > 255 {
			c.JSON(400, gin.H{"error": "valid type_id, ref_value and name (at most 255 bytes) required"})
			return
		}
		typ, err := types.GetProfileTypeByID(c.Request.Context(), input.TypeID)
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, serror.ErrEntityProfileTypeNotFound) || (err == nil && (typ == nil || typ.CompanyID != company || typ.ArchivedAt != nil)) {
			c.JSON(404, gin.H{"error": "profile type not found"})
			return
		}
		if err != nil {
			c.JSON(500, gin.H{"error": "could not load profile type"})
			return
		}
		profile := &domain.EntityProfile{ID: uuid.NewString(), CompanyID: company, ProfileTypeID: typ.ID, RefKey: input.Ref, Name: input.Name}
		if err = profiles.CreateProfile(c.Request.Context(), profile); err != nil {
			status := 500
			message := "could not save profile"
			if errors.Is(err, registry.ErrLimit) {
				status = 429
				message = "entity profile license limit reached"
			}
			if errors.Is(err, registry.ErrUnverified) {
				status = 503
				message = "license verification unavailable"
			}
			c.JSON(status, gin.H{"error": message})
			return
		}
		profile, err = profiles.GetProfileByRefKey(c.Request.Context(), company, typ.ID, input.Ref)
		if err != nil {
			c.JSON(500, gin.H{"error": "profile saved but could not read result"})
			return
		}
		c.JSON(200, gin.H{"data": gin.H{"id": profile.ID, "company_id": company, "profile_type_id": typ.ID, "ref_key": profile.RefKey, "name": profile.Name, "created_at": profile.CreatedAt, "last_active_at": profile.LastActiveAt}})
	}
}
