package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/threadify/engine/internal/database"
	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/models"
	"github.com/threadify/engine/internal/repository/postgres"
	"github.com/threadify/engine/internal/repository/valkey"
)

type ThreadService struct {
	repo              interfaces.ThreadRepository
	graphRepo         interfaces.ContractGraphRepository
	stepEventService  interfaces.StepEventProcessor
	cacheManager      interfaces.CacheManager
	connectionMgr     interfaces.ConnectionManager
	contractValidator interfaces.ContractValidator
	authService       *AuthService
	accessService     *ThreadAccessService
	valkeyClient      interfaces.ValkeyClient
	luaScripts        *LuaScriptManager
}

func NewThreadService(repo interfaces.ThreadRepository, graphRepo interfaces.ContractGraphRepository, stepEventService interfaces.StepEventProcessor, cacheManager interfaces.CacheManager, connectionMgr interfaces.ConnectionManager, contractValidator interfaces.ContractValidator, accessService *ThreadAccessService) *ThreadService {
	return &ThreadService{
		repo:              repo,
		graphRepo:         graphRepo,
		stepEventService:  stepEventService,
		cacheManager:      cacheManager,
		connectionMgr:     connectionMgr,
		contractValidator: contractValidator,
		authService:       NewAuthService("demo-secret", "threadify", "threadify-api", 24),
		accessService:     accessService,
	}
}

// NewThreadServiceWithDefaults creates ThreadService with concrete implementations (for production)
func NewThreadServiceWithDefaults(db *database.PostgresDB, valkeyService *database.ValkeyService, stepEventService *StepEventService, contractTTLSeconds, threadTTLSeconds int) *ThreadService {
	// Create cache service first
	cacheService := NewCacheService()

	// Create repositories
	contractRepo := postgres.NewContractRepository(db.Pool)
	valkeyGraphRepo := valkey.NewContractGraphRepository(valkeyService, contractTTLSeconds) // Configurable TTL for graphs
	threadRepo := valkey.NewThreadRepository(valkeyService, threadTTLSeconds)

	// Create thread access service for permission/role management
	accessService := NewThreadAccessService(threadRepo, cacheService)

	// Create and load Lua scripts
	luaScripts := NewLuaScriptManager(valkeyService)
	if err := luaScripts.LoadScripts(context.Background()); err != nil {
		fmt.Printf("Warning: Failed to load Lua scripts: %v\n", err)
	}

	service := NewThreadService(
		threadRepo,      // Valkey thread repository
		valkeyGraphRepo, // Valkey contract graph repository
		stepEventService,
		cacheService,           // In-memory cache service
		NewConnectionService(), // In-memory connection service
		NewContractValidationService(valkeyGraphRepo, contractRepo, cacheService), // Contract validation service with three-tier caching
		accessService, // Thread access service for permissions/roles
	)
	service.valkeyClient = valkeyService
	service.luaScripts = luaScripts

	return service
}

func (s *ThreadService) HandleConnect(req *models.ConnectRequest) *models.ConnectResponse {
	if req.ApiKey == "" {
		return &models.ConnectResponse{
			Action:  "connect",
			Status:  "error",
			Message: "API key is required",
		}
	}

	// Validate API key and derive user information
	userInfo, err := s.authService.ValidateApiKey(req.ApiKey)
	if err != nil {
		return &models.ConnectResponse{
			Action:  "connect",
			Status:  "error",
			Message: fmt.Sprintf("Invalid API key: %v", err),
		}
	}

	// Connect with derived owner and company information
	err = s.connectionMgr.ConnectWithOwnerAndCompany(userInfo.OwnerID, req.ApiKey, req.ServiceName, userInfo.CompanyID)
	if err != nil {
		return &models.ConnectResponse{
			Action:  "connect",
			Status:  "error",
			Message: fmt.Sprintf("Failed to connect: %v", err),
		}
	}

	return &models.ConnectResponse{
		Action:    "connect",
		Status:    "success",
		Message:   "Connected successfully",
		OwnerID:   userInfo.OwnerID,
		CompanyID: userInfo.CompanyID,
	}
}

func (s *ThreadService) HandleStartThread(req *models.StartThreadRequest, ownerID string, companyID string) *models.StartThreadResponse {
	if ownerID == "" || !s.connectionMgr.IsConnected(ownerID) {
		return &models.StartThreadResponse{
			Action:  "startThread",
			Status:  "error",
			Message: "Not authenticated. Please connect first.",
		}
	}

	// Validate required fields (only for contract-based workflows)
	if req.ContractName != "" && req.Role == "" {
		return &models.StartThreadResponse{
			Action:  "startThread",
			Status:  "error",
			Message: "Role is required when contract name is provided",
		}
	}

	// Parse contract identifier and load contract graph if contract name provided
	var contractVersion int = 0
	var parsedContractName string
	if req.ContractName != "" {
		parsedContractName, contractVersion = parseContractIdentifier(req.ContractName)

		// Load contract graph with parsed name and version
		if err := s.contractValidator.LoadContractGraphIntoCache(parsedContractName, contractVersion); err != nil {
			return &models.StartThreadResponse{
				Action:  "startThread",
				Status:  "error",
				Message: fmt.Sprintf("Failed to load contract: %v", err),
			}
		}
	}

	threadID := uuid.New().String()

	// Create thread with company information (supports non-contract workflows)
	thread := models.NewThreadWithCompany(threadID, parsedContractName, contractVersion, ownerID, companyID)
	thread.ContractName = req.ContractName // Keep original format for reference
	thread.Refs = req.Refs

	// Save thread to repository and cache
	if err := s.repo.Save(context.Background(), thread); err != nil {
		return &models.StartThreadResponse{
			Action:  "startThread",
			Status:  "error",
			Message: fmt.Sprintf("Failed to create thread: %v", err),
		}
	}

	// Cache the thread for fast access
	s.cacheManager.SetThread(threadID, thread)

	// Assign role to user for this thread if role is provided
	if req.Role != "" {
		if err := s.accessService.AssignRole(threadID, req.Role, ownerID); err != nil {
			// Log error but don't fail thread creation
			fmt.Printf("Warning: failed to assign role: %v\n", err)
		}
	}

	// Write thread metadata to stream for archival (async, don't fail if it fails)
	go func() {
		ctx := context.Background()

		// Convert contract version to string (handle nil pointer)
		contractVersion := "0"
		if thread.ContractVersion != nil {
			contractVersion = fmt.Sprintf("%d", *thread.ContractVersion)
		}

		// Handle nil contract ID
		contractID := ""
		if thread.ContractID != nil {
			contractID = *thread.ContractID
		}

		streamValues := map[string]interface{}{
			"id":              threadID,
			"ownerId":         ownerID,
			"companyId":       companyID,
			"contractId":      contractID,
			"contractVersion": contractVersion,
			"contractName":    thread.ContractName,
			"status":          thread.Status,
			"lastHash":        thread.LastHash,
			"startedAt":       thread.StartedAt.Format(time.RFC3339),
			"completedAt":     "",
			"maxlen":          "~",
			"limit":           100000,
		}
		s.valkeyClient.XAdd(ctx, "streams:thread_metadata", streamValues)
	}()

	return &models.StartThreadResponse{
		Action:   "startThread",
		Status:   "success",
		Message:  "Thread started successfully",
		ThreadID: threadID,
	}
}

// hasSuccessfulSteps checks if thread has any completed steps
func (s *ThreadService) hasSuccessfulSteps(thread *models.Thread) bool {
	// Check if current_steps sorted set has any members
	currentStepsKey := fmt.Sprintf("thread:%s:current_steps", thread.ID)
	count, err := s.valkeyClient.ZCard(context.Background(), currentStepsKey)
	if err != nil {
		return false
	}
	return count > 0
}

func (s *ThreadService) HandleRecordEvent(req *models.RecordEventRequest, ownerID string, companyID string) *models.RecordEventResponse {
	if ownerID == "" || !s.connectionMgr.IsConnected(ownerID) {
		return &models.RecordEventResponse{
			Action:  "recordThreadEvent",
			Status:  "error",
			Message: "Not authenticated. Please connect first.",
		}
	}

	// Validate required fields
	if req.ThreadID == "" {
		return &models.RecordEventResponse{
			Action:  "recordThreadEvent",
			Status:  "error",
			Message: "Thread ID is required",
		}
	}
	if req.StepName == "" {
		return &models.RecordEventResponse{
			Action:  "recordThreadEvent",
			Status:  "error",
			Message: "StepName is required",
		}
	}
	if req.Status == "" {
		return &models.RecordEventResponse{
			Action:  "recordThreadEvent",
			Status:  "error",
			Message: "Status is required",
		}
	}
	if req.StartedAt == "" {
		return &models.RecordEventResponse{
			Action:  "recordThreadEvent",
			Status:  "error",
			Message: "StartedAt is required",
		}
	}
	if req.FinishedAt == "" {
		return &models.RecordEventResponse{
			Action:  "recordThreadEvent",
			Status:  "error",
			Message: "FinishedAt is required",
		}
	}
	if req.Context == nil {
		return &models.RecordEventResponse{
			Action:  "recordThreadEvent",
			Status:  "error",
			Message: "Context is required",
		}
	}

	// Get thread
	thread, err := s.getThread(req.ThreadID)
	if err != nil {
		return &models.RecordEventResponse{
			Action:  "recordThreadEvent",
			Status:  "error",
			Message: fmt.Sprintf("Thread not found: %s", req.ThreadID),
		}
	}

	// Check permission - user must have write access
	hasAccess, err := s.accessService.CheckThreadAccess(req.ThreadID, ownerID, "write", thread)
	if err != nil || !hasAccess {
		return &models.RecordEventResponse{
			Action:  "recordThreadEvent",
			Status:  "error",
			Message: "Access denied: You don't have write permission for this thread",
		}
	}

	// Check thread status - cannot add steps to completed threads
	if thread.Status == "completed" {
		return &models.RecordEventResponse{
			Action:  "recordThreadEvent",
			Status:  "error",
			Message: "Cannot add steps to completed thread",
		}
	}

	// Generate idempotency key from context hash if not provided
	idempotencyKey := req.IdempotencyKey
	if idempotencyKey == "" && req.Context != nil && len(req.Context) > 0 {
		// Generate deterministic hash from context fields
		idempotencyKey = generateContextHash(req.Context)
		fmt.Printf("[IDEMPOTENCY] Auto-generated key from context hash: %s\n", idempotencyKey)
	}

	// Check for duplicate step using idempotency key
	if idempotencyKey != "" {
		stepKey := req.StepName + ":" + idempotencyKey
		stepHashKey := fmt.Sprintf("thread:%s:steps:%s", req.ThreadID, stepKey)

		// Check if step state hash exists
		existingStatus, err := s.valkeyClient.HGet(context.Background(), stepHashKey, "status")
		if err == nil && existingStatus != "" {
			// Step exists - check if it's already completed
			if existingStatus == "completed" {
				// Duplicate successful step - reject
				return &models.RecordEventResponse{
					Action:      "recordThreadEvent",
					Status:      "error",
					Message:     "Step with this signature already completed",
					IsDuplicate: true,
				}
			}
			// If status is "pending", "violated", or "failed", allow retry
		}
	}

	// Update request with generated idempotency key for downstream processing
	if req.IdempotencyKey == "" && idempotencyKey != "" {
		req.IdempotencyKey = idempotencyKey
	}

	// Declare variables for async validation
	var graph *models.ContractGraph
	var stepNode models.GraphNode

	// Validate contract if thread has one
	if thread.ContractName != "" {
		// Determine version to use (0 means latest)
		version := 0
		if thread.ContractVersion != nil {
			version = *thread.ContractVersion
		}

		// Get contract graph (three-tier cached)
		var err error
		graph, err = s.contractValidator.GetContractGraph(thread.ContractName, version)
		if err != nil {
			return &models.RecordEventResponse{
				Action:  "recordThreadEvent",
				Status:  "error",
				Message: fmt.Sprintf("Failed to load contract: %v", err),
			}
		}

		// Check if step exists in contract
		var exists bool
		stepNode, exists = graph.Graph.Nodes[req.StepName]
		if !exists {
			return &models.RecordEventResponse{
				Action:  "recordThreadEvent",
				Status:  "error",
				Message: fmt.Sprintf("Step '%s' not found in contract '%s'", req.StepName, thread.ContractName),
			}
		}

		// Validate entry point - thread must start with an entry point
		if !s.hasSuccessfulSteps(thread) {
			// This is the first step - must be an entry point
			isEntryPoint := false
			for _, entryPoint := range graph.Graph.EntryPoints {
				if entryPoint == req.StepName {
					isEntryPoint = true
					break
				}
			}

			if !isEntryPoint {
				return &models.RecordEventResponse{
					Action:  "recordThreadEvent",
					Status:  "error",
					Message: fmt.Sprintf("Thread must start with one of the entry points: %v. Attempted step: '%s'", graph.Graph.EntryPoints, req.StepName),
				}
			}
		}

		// Validate role if step has an owner requirement
		if stepNode.Owner != "" {
			hasRole, err := s.accessService.ValidateUserRoleForStep(req.ThreadID, ownerID, stepNode.Owner)
			if err != nil || !hasRole {
				userRole, _ := s.accessService.GetUserRole(req.ThreadID, ownerID)
				return &models.RecordEventResponse{
					Action:  "recordThreadEvent",
					Status:  "error",
					Message: fmt.Sprintf("Access denied: Step '%s' requires owner '%s', you have role '%s'", req.StepName, stepNode.Owner, userRole),
				}
			}
		}

		// Validate step context against contract (using already-fetched stepNode)
		if validationErr := s.contractValidator.ValidateStepContext(stepNode, req.Context); validationErr != nil {
			return &models.RecordEventResponse{
				Action:  "recordThreadEvent",
				Status:  "error",
				Message: fmt.Sprintf("Step validation failed: %v", validationErr),
			}
		}
	}

	// Generate step ID (UUID)
	stepID := uuid.New().String()

	// Get service name from request or connected client
	serviceName := req.ServiceName
	if serviceName == "" {
		if client, exists := s.connectionMgr.GetClient(ownerID); exists {
			serviceName = client.ServiceName
		}
	}
	if serviceName == "" {
		serviceName = "unknown"
	}

	// Convert RecordEventRequest to StepEvent for cryptographic processing
	// Convert context from map[string]string to map[string]interface{}
	contextInterface := make(map[string]interface{})
	for k, v := range req.Context {
		contextInterface[k] = v
	}

	// Create step event for processing
	stepEvent := &models.StepEvent{
		StepID:      stepID, // Use StepID instead of ID
		ThreadID:    req.ThreadID,
		StepName:    req.StepName,
		ServiceName: serviceName,
		Type:        req.Type, // Use Type from request
		Status:      req.Status,
		Context:     contextInterface, // Use converted context
		StartedAt:   req.StartedAt,
		FinishedAt:  req.FinishedAt,
		Timestamp:   time.Now(), // Use Timestamp instead of CreatedAt
	}

	// Process step event immediately
	if err := s.stepEventService.ProcessStepEvent(*stepEvent); err != nil {
		return &models.RecordEventResponse{
			Action:  "recordThreadEvent",
			Status:  "error",
			Message: fmt.Sprintf("Failed to process step event: %v", err),
		}
	}

	// Trigger async validation for ALL threads (contract or not) with successful steps
	// The async validation will update step state via Lua script
	if req.Status == "success" {
		s.performAsyncValidation(req.ThreadID, stepID, req.StepName, ownerID, req, thread, graph, stepNode)
	}

	return &models.RecordEventResponse{
		Action:   "recordThreadEvent",
		Status:   "success",
		Message:  "Step Event recorded successfully",
		ThreadID: req.ThreadID,
		StepID:   stepID,
	}
}

func (s *ThreadService) HandleClose(ownerID string) *models.CloseConnectionResponse {
	if ownerID != "" {
		s.connectionMgr.Disconnect(ownerID)
	}

	return &models.CloseConnectionResponse{
		Action:  "closeConnection",
		Status:  "success",
		Message: "Connection closed successfully",
	}
}

// parseContractIdentifier parses a contract identifier which can be:
//   - "contract_name" -> returns ("contract_name", 0)
//   - "contract_name:2" -> returns ("contract_name", 2)
//   - "uuid-string" -> returns ("uuid-string", 0)
func parseContractIdentifier(identifier string) (name string, version int) {
	// Check if identifier contains a colon
	if idx := strings.LastIndex(identifier, ":"); idx != -1 {
		// Split into name and version parts
		name = identifier[:idx]
		versionStr := identifier[idx+1:]

		// Try to parse version as integer
		if v, err := strconv.Atoi(versionStr); err == nil && v > 0 {
			return name, v
		}

		// If version parsing fails, treat the whole thing as a name
		return identifier, 0
	}

	// No colon found, return as-is with no version
	return identifier, 0
}

// Helper function to convert map[string]string to map[string]interface{}
func convertStringMapToInterfaceMap(stringMap map[string]string) map[string]interface{} {
	if stringMap == nil {
		return make(map[string]interface{})
	}

	interfaceMap := make(map[string]interface{})
	for k, v := range stringMap {
		interfaceMap[k] = v
	}
	return interfaceMap
}

// getThread retrieves thread from cache first, then Valkey as fallback
func (s *ThreadService) getThread(threadID string) (*models.Thread, error) {
	// Check cache first
	if thread, exists := s.cacheManager.GetThread(threadID); exists {
		return thread, nil
	}

	// Load from Valkey if not in cache
	thread, err := s.repo.Get(context.Background(), threadID)
	if err != nil {
		return nil, fmt.Errorf("failed to load thread: %w", err)
	}

	// Cache the thread for future use
	s.cacheManager.SetThread(threadID, thread)
	return thread, nil
}

// AssignThreadRole assigns a role to a user in a thread
func (s *ThreadService) AssignThreadRole(threadID, role, userID string) error {
	return s.accessService.AssignRole(threadID, role, userID)
}

// SetThreadPermissions sets permissions for a user in a thread
func (s *ThreadService) SetThreadPermissions(threadID, userID string, permissions []string) error {
	return s.accessService.SetUserPermissions(threadID, userID, permissions)
}

// generateContextHash creates a deterministic hash from context fields
// This enables automatic retry tracking without explicit idempotency keys
func generateContextHash(context map[string]string) string {
	if len(context) == 0 {
		return ""
	}

	// Sort keys for deterministic ordering
	keys := make([]string, 0, len(context))
	for k := range context {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build deterministic string from sorted key-value pairs
	var builder strings.Builder
	for _, k := range keys {
		builder.WriteString(k)
		builder.WriteString("=")
		builder.WriteString(context[k])
		builder.WriteString(";")
	}

	// Generate SHA256 hash
	hash := sha256.Sum256([]byte(builder.String()))

	// Return first 16 characters of hex encoding (sufficient for uniqueness)
	return hex.EncodeToString(hash[:])[:16]
}
