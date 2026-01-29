package service

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"threadify-go/shared/rbac"

	"github.com/google/uuid"
	"github.com/threadify/engine/internal/config"
	"github.com/threadify/engine/internal/database"
	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/models"
	natsrepo "github.com/threadify/engine/internal/repository/nats"
	"github.com/threadify/engine/internal/repository/valkey"
	"github.com/threadify/engine/internal/utils"
)

// ThreadService orchestrates thread operations across multiple repositories
type ThreadService struct {
	repo                  interfaces.ThreadRepository
	accessRepo            interfaces.AccessRepository
	activityRepo          interfaces.ActivityRepository
	graphRepo             interfaces.ContractGraphRepository
	stepEventService      interfaces.StepEventProcessor
	cacheManager          interfaces.CacheManager
	connectionMgr         interfaces.ConnectionManager
	contractValidator     interfaces.ContractValidator
	authService           *AuthService
	accessService         *ThreadAccessService
	validationService     *ValidationService
	notificationService   *NotificationService
	invitationService     *InvitationTokenService
	scopeResolver         *ScopeResolver
	notificationConsumer  *NotificationConsumer
	valkeyClient          interfaces.ValkeyClient
	luaScripts            *valkey.LuaScriptManager
	natsArchivalPublisher *natsrepo.ArchivalPublisher
	rbacLoader            *rbac.Loader
}

// NewThreadService creates ThreadService with all dependencies using builder pattern
// This is the main constructor used in production
// natsPublisher and natsArchivalPublisher can be nil for graceful degradation
func NewThreadService(cfg *config.Config, db *database.PostgresDB, valkeyService *database.ValkeyService, stepEventService *StepEventService, threadRepo *valkey.ThreadRepository, contractTTLSeconds int, natsPublisher NotificationPublisher, natsArchivalPublisher *natsrepo.ArchivalPublisher, authService *AuthService) *ThreadService {
	service, err := NewThreadServiceBuilder().
		WithConfig(cfg).
		WithDatabase(db).
		WithValkey(valkeyService).
		WithStepEventService(stepEventService).
		WithThreadRepository(threadRepo).
		WithContractTTL(contractTTLSeconds).
		WithNATSPublisher(natsPublisher).
		WithNATSArchivalPublisher(natsArchivalPublisher).
		WithAuthService(authService).
		Build()

	if err != nil {
		// Fallback to panic since this is a critical initialization error
		panic(fmt.Sprintf("Failed to build ThreadService: %v", err))
	}

	return service
}

// GetNotificationConsumer returns the notification consumer (can be nil)
func (s *ThreadService) GetNotificationConsumer() *NotificationConsumer {
	return s.notificationConsumer
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
	start := time.Now()
	log.Printf("[PERF] HandleStartThread BEGIN: owner=%s, contract=%s", ownerID, req.ContractName)

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
	var contractUUID string
	if req.ContractName != "" {
		parsedContractName, contractVersion = parseContractIdentifier(req.ContractName)

		// Load contract graph with parsed name and version (uses company_id internally)
		// This returns the actual version loaded (resolves version 0 to latest)
		actualVersion, err := s.contractValidator.LoadContractGraphIntoCache(parsedContractName, contractVersion, companyID)
		if err != nil {
			return &models.StartThreadResponse{
				Action:  "startThread",
				Status:  "error",
				Message: fmt.Sprintf("Failed to load contract: %v", err),
			}
		}
		contractVersion = actualVersion // Use the actual version that was loaded

		// Get contract UUID for referential integrity
		contract, err := s.contractValidator.GetContractByNameAndCompany(parsedContractName, companyID)
		if err != nil {
			return &models.StartThreadResponse{
				Action:  "startThread",
				Status:  "error",
				Message: fmt.Sprintf("Failed to retrieve contract: %v", err),
			}
		}
		contractUUID = contract.ID
	}

	threadID := uuid.New().String()

	// Create thread with company information (supports non-contract workflows)
	var contractIDPtr *string
	if contractUUID != "" {
		contractIDPtr = &contractUUID // Store UUID for referential integrity
	}

	// Set contract version pointer if contract is used
	var contractVersionPtr *int
	if parsedContractName != "" && contractVersion > 0 {
		contractVersionPtr = &contractVersion
	}

	thread := &models.Thread{
		ID:              threadID,
		ContractID:      contractIDPtr,      // Store contract UUID for referential integrity
		ContractName:    parsedContractName, // Store name for display/filtering
		ContractVersion: contractVersionPtr, // Store actual version that was loaded
		OwnerID:         ownerID,
		CompanyID:       companyID,
		Status:          "active",
		StartedAt:       time.Now(),
	}

	// Prepare creator access
	creatorRole := req.Role
	if creatorRole == "" {
		creatorRole = "owner" // Default thread role if not specified
	}

	// OPTIMIZATION: Atomic thread creation + access grant in single operation
	// This combines repo.Save() and GrantOrUpdateAccess() to reduce roundtrips
	createCtx, createCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer createCancel()

	// Resolve runtime_role for creator
	// CRITICAL: Creator must always get "owner" runtime_role
	// If this fails, it indicates a fundamental system error
	runtimeRole, err := s.scopeResolver.ResolveScope(
		createCtx,
		threadID,
		ownerID,
		creatorRole,
		true, // isCreator
		nil,  // explicitScope
	)
	if err != nil {
		log.Printf("[CRITICAL] Failed to resolve runtime_role for creator %s in thread %s: %v", ownerID, threadID, err)
		return &models.StartThreadResponse{
			Action:  "startThread",
			Status:  "error",
			Message: "System error: failed to assign creator permissions",
		}
	}

	// Sanity check: creator must always be "owner"
	if runtimeRole != "owner" {
		log.Printf("[CRITICAL] Creator %s got runtime_role '%s' instead of 'owner' for thread %s", ownerID, runtimeRole, threadID)
		return &models.StartThreadResponse{
			Action:  "startThread",
			Status:  "error",
			Message: "System error: invalid creator permissions",
		}
	}

	// Serialize thread data for atomic creation
	threadDataBytes, err := thread.ToJSON()
	if err != nil {
		return &models.StartThreadResponse{
			Action:  "startThread",
			Status:  "error",
			Message: fmt.Sprintf("Failed to serialize thread: %v", err),
		}
	}
	threadDataStr := string(threadDataBytes)

	// Atomic thread creation + access grant via service layer
	// This combines both operations into a single Valkey roundtrip
	// Service layer resolves permissions from runtime_role before storing
	// TTL: 5 hours (18000 seconds) - matches config default
	threadTTLSeconds := 18000
	access, err := s.accessService.GrantAccessWithThreadCreation(
		createCtx,
		threadID,
		ownerID,
		creatorRole,       // Thread role (e.g., "merchant", "supplier")
		runtimeRole,       // Runtime-level permission scope (e.g., "owner")
		&threadDataStr,    // Pass thread data for atomic creation
		&threadTTLSeconds, // Pass TTL in seconds
	)
	if err != nil {
		return &models.StartThreadResponse{
			Action:  "startThread",
			Status:  "error",
			Message: fmt.Sprintf("Failed to create thread: %v", err),
		}
	}

	// Cache the thread for fast access
	s.cacheManager.SetThread(threadID, thread)

	// Record activity asynchronously (don't block)
	go s.recordThreadCreationActivity(threadID, ownerID, access, runtimeRole)

	// Write thread metadata to stream for archival (async, don't fail if it fails)
	go s.publishThreadMetadataAsync(threadID, ownerID, companyID, thread, req.Role)

	totalDuration := time.Since(start)
	log.Printf("[PERF] HandleStartThread COMPLETE: duration=%v | success=true", totalDuration)

	return &models.StartThreadResponse{
		Action:   "startThread",
		Status:   "success",
		Message:  "Thread started successfully",
		ThreadID: threadID,
	}
}

// hasSuccessfulSteps checks if thread has any completed steps
func (s *ThreadService) hasSuccessfulSteps(thread *models.Thread) bool {
	// Use repository method instead of direct Valkey call
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count, err := s.repo.GetCompletedStepsCount(ctx, thread.ID, true)
	if err != nil {
		return false
	}
	return count > 0
}

func (s *ThreadService) HandleRecordEvent(req *models.RecordEventRequest, ownerID string, companyID string) *models.RecordEventResponse {
	start := time.Now()
	log.Printf("[PERF] HandleRecordEvent BEGIN: owner=%s, thread=%s, step=%s", ownerID, req.ThreadID[:8], req.StepName)

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
	thread, err := s.GetThread(req.ThreadID)
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
		idempotencyKey = utils.GenerateContextHash(req.Context)
		fmt.Printf("[IDEMPOTENCY] Auto-generated key from context hash: %s\n", idempotencyKey)
	}

	// Check for duplicate step using idempotency key
	if idempotencyKey != "" {
		// Use repository method instead of direct Valkey call
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		existingStatus, err := s.repo.GetStepStatus(ctx, req.ThreadID, req.StepName, idempotencyKey, true)
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

		// Get contract graph (three-tier cached) using company_id
		var err error
		graph, err = s.contractValidator.GetContractGraph(thread.ContractName, version, thread.CompanyID)
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
			// Security: Ensure the step owner role is actually defined in contract parties
			if len(graph.Parties) > 0 && !isRoleInParties(stepNode.Owner, graph.Parties) {
				return &models.RecordEventResponse{
					Action:  "recordThreadEvent",
					Status:  "error",
					Message: fmt.Sprintf("Access denied: Step '%s' requires owner role '%s' which is not defined in contract parties: %v", req.StepName, stepNode.Owner, graph.Parties),
				}
			}

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
		StepID:         stepID, // Use StepID instead of ID
		ThreadID:       req.ThreadID,
		StepName:       req.StepName,
		ServiceName:    serviceName,
		Type:           req.Type, // Use Type from request
		Status:         req.Status,
		Context:        contextInterface, // Use converted context
		StartedAt:      req.StartedAt,
		FinishedAt:     req.FinishedAt,
		Timestamp:      time.Now(),         // Use Timestamp instead of CreatedAt
		IdempotencyKey: req.IdempotencyKey, // Pass through idempotency key (user-provided or auto-generated)
	}

	// Process step event immediately
	if err := s.stepEventService.RecordStepEventDirect(*stepEvent, ownerID, serviceName); err != nil {
		return &models.RecordEventResponse{
			Action:  "recordThreadEvent",
			Status:  "error",
			Message: fmt.Sprintf("Failed to process step event: %v", err),
		}
	}

	// Trigger async validation for ALL threads (contract or not) with successful or failed steps
	// The async validation will update step state via Lua script and check retry limits
	if req.Status == "success" || req.Status == "failed" || req.Status == "error" {
		s.notificationService.PerformAsyncValidation(req.ThreadID, stepID, req.StepName, ownerID, req, thread, graph, stepNode)
	}

	totalDuration := time.Since(start)
	log.Printf("[PERF] HandleRecordEvent COMPLETE: duration=%v | success=true", totalDuration)

	return &models.RecordEventResponse{
		Action:   "recordThreadEvent",
		Status:   "success",
		Message:  "Step Event recorded successfully",
		ThreadID: req.ThreadID,
		StepID:   stepID,
	}
}

// HandleInviteParty creates invitation tokens for thread access
func (s *ThreadService) HandleInviteParty(req *models.InvitePartyRequest, ownerID, companyID string, threadIDs []string) (*models.InvitePartyResponse, error) {
	// Set default permissions if not provided
	permissions := req.Permissions
	if permissions == "" {
		permissions = "read,write"
	}

	// Convert comma-separated string to slice for CreateToken
	permissionsSlice := strings.Split(permissions, ",")
	for i := range permissionsSlice {
		permissionsSlice[i] = strings.TrimSpace(permissionsSlice[i])
	}

	// Parse expiry
	expiry, err := s.invitationService.ParseExpiry(req.ExpiresIn)
	if err != nil {
		return nil, fmt.Errorf("invalid expiry format: %v", err)
	}

	// Get thread context from session's threads
	// For now, we'll use the first thread in the session's threadIDs
	var threadID string
	if len(threadIDs) > 0 {
		threadID = threadIDs[0] // Use first available thread
	}

	if threadID == "" {
		return nil, fmt.Errorf("no active thread found. Please start a thread first")
	}

	// Get thread to access its contract
	thread, err := s.GetThread(threadID)
	if err != nil {
		return nil, fmt.Errorf("thread not found")
	}

	// Get contract graph to validate role exists in contract parties
	contractGraph, err := s.GetContractGraphForThread(thread)
	if err != nil {
		log.Printf("Failed to get contract graph for thread %s: %v", threadID, err)
		return nil, fmt.Errorf("failed to load contract configuration")
	}

	// Validate role exists in contract parties
	if len(contractGraph.Parties) > 0 {
		roleInParties := false
		for _, party := range contractGraph.Parties {
			if party == req.Role {
				roleInParties = true
				break
			}
		}
		if !roleInParties {
			return nil, fmt.Errorf("role '%s' is not defined in contract parties: %v", req.Role, contractGraph.Parties)
		}
	}

	// Validate permissions
	if err := s.invitationService.ValidatePermissions(permissions); err != nil {
		return nil, err
	}

	// Create JWT token
	threadToken, err := s.invitationService.CreateToken(threadID, ownerID, req.Role, permissionsSlice, expiry)
	if err != nil {
		log.Printf("Failed to create invitation token: %v", err)
		return nil, fmt.Errorf("failed to create invitation token")
	}

	return &models.InvitePartyResponse{
		Action:      "inviteParty",
		Status:      "success",
		ThreadToken: threadToken,
		Role:        req.Role,
		Permissions: permissions,
		ExpiresAt:   time.Now().Add(expiry).Unix(),
		Message:     "Invitation token created successfully",
	}, nil
}

// HandleJoinThread handles both token-based and direct thread joining
func (s *ThreadService) HandleJoinThread(req *models.JoinThreadRequest, ownerID, companyID string) (*models.JoinThreadResponse, error) {
	var threadID, role, invitedBy string
	var thread *models.Thread

	// Mode 1: Token-based join (invitation)
	if req.ThreadToken != "" {
		// Validate JWT token and extract claims
		claims, err := s.invitationService.ValidateToken(req.ThreadToken)
		if err != nil {
			return nil, fmt.Errorf("invalid thread token")
		}

		threadID = claims.ThreadID
		role = claims.Role
		invitedBy = claims.InvitedBy

		// Mode 2: Direct join (same company, no token)
	} else if req.ThreadID != "" {
		// Set default role if not provided
		if req.Role == "" {
			req.Role = "participant" // Default role for direct join
		}
		// Get thread to validate it exists and check company
		var err error
		thread, err = s.GetThread(req.ThreadID)
		if err != nil {
			return nil, fmt.Errorf("thread not found")
		}

		// Validate same company
		if thread.CompanyID != companyID {
			return nil, fmt.Errorf("can only join threads from same company")
		}

		// Validate role
		if !s.IsValidRole(req.Role) {
			return nil, fmt.Errorf("invalid role: %s", req.Role)
		}

		threadID = req.ThreadID
		role = req.Role
		invitedBy = companyID // Company ID as inviter

	} else {
		return nil, fmt.Errorf("either threadToken or (threadId + role) is required")
	}

	// Get thread if not already loaded (for status validation)
	if thread == nil {
		var err error
		thread, err = s.GetThread(threadID)
		if err != nil {
			return nil, fmt.Errorf("thread not found")
		}
	}

	// Validate role is in contract parties (if parties are defined)
	contractGraph, err := s.GetContractGraphForThread(thread)
	if err == nil && len(contractGraph.Parties) > 0 {
		roleInParties := false
		for _, party := range contractGraph.Parties {
			if party == role {
				roleInParties = true
				break
			}
		}
		if !roleInParties {
			return nil, fmt.Errorf("role '%s' is not defined in contract parties: %v", role, contractGraph.Parties)
		}
	}

	// Grant or update access using unified method
	// For join: not creator, no explicit scope (will use contract defaults or system default)
	err = s.GrantOrUpdateThreadAccess(threadID, ownerID, role, invitedBy, false, nil)
	if err != nil {
		log.Printf("Failed to grant access for user %s to thread %s: %v", ownerID, threadID, err)
		return nil, fmt.Errorf("failed to grant access")
	}

	return &models.JoinThreadResponse{
		Action:   "joinThread",
		Status:   "success",
		ThreadID: threadID,
		Role:     role,
		Message:  "Successfully joined thread",
	}, nil
}

// HandleClose removes the owner from the connection manager and closes the connection
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

// GetThread retrieves thread from cache first, then Valkey with PostgreSQL fallback
func (s *ThreadService) GetThread(threadID string) (*models.Thread, error) {
	// Check in-memory cache first (fastest)
	if thread, exists := s.cacheManager.GetThread(threadID); exists {
		return thread, nil
	}

	// Load from Valkey (hot) or PostgreSQL (cold) with write-back enabled
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	thread, err := s.repo.Get(ctx, threadID, true) // writeBack=true to cache PostgreSQL data in Valkey
	if err != nil {
		return nil, fmt.Errorf("failed to load thread: %w", err)
	}

	// Cache the thread in memory for future use
	s.cacheManager.SetThread(threadID, thread)
	return thread, nil
}

// GrantOrUpdateThreadAccess grants or updates user access using unified method with proper orchestration
func (s *ThreadService) GrantOrUpdateThreadAccess(threadID, userID, role string, invitedBy string, isCreator bool, explicitScope *string) error {
	// 1. Resolve runtime_role (notification/permission scope)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	runtimeRole, err := s.scopeResolver.ResolveScope(
		ctx,
		threadID,
		userID,
		role,
		isCreator,
		explicitScope,
	)
	if err != nil {
		log.Printf("[ERROR] Failed to resolve runtime_role for user %s in thread %s: %v", userID, threadID, err)
		return fmt.Errorf("failed to resolve runtime_role: %w", err)
	}

	// Validate runtime_role is not empty (unless explicitly set to empty via explicitScope)
	if runtimeRole == "" && (explicitScope == nil || *explicitScope != "") {
		log.Printf("[ERROR] Empty runtime_role resolved for user %s in thread %s (isCreator=%v, role=%s)",
			userID, threadID, isCreator, role)
		return fmt.Errorf("invalid runtime_role: cannot be empty")
	}

	// 2. Grant access via ThreadAccessService (resolves permissions automatically)
	err = s.accessService.GrantOrUpdateAccess(threadID, userID, []string{role}, runtimeRole, invitedBy)
	if err != nil {
		return fmt.Errorf("failed to grant access: %w", err)
	}

	// Get the access object for activity recording
	accessCtx, accessCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer accessCancel()
	access, err := s.accessRepo.GetUserAccess(accessCtx, threadID, userID)
	if err != nil {
		log.Printf("Failed to get access for activity recording: %v", err)
		// Don't fail the operation, just log it
		access = nil
	}

	// 3. Record activity via ActivityRepository (async, don't block main operation)
	// This is where runtime_role gets persisted to PostgreSQL via NATS archival
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		// Get service name from connection manager
		client, exists := s.connectionMgr.GetClient(userID)
		serviceName := ""
		if exists {
			serviceName = client.ServiceName
		}
		if err := s.activityRepo.RecordAccessGranted(ctx, threadID, userID, access, invitedBy, serviceName, runtimeRole); err != nil {
			log.Printf("Failed to record access granted activity: %v", err)
		}
	}()

	return nil
}

// IsValidRole validates if a role name is valid runtime-level role
func (s *ThreadService) IsValidRole(role string) bool {
	if role == "" {
		return false
	}

	// If RBAC loader is not available, fall back to basic validation
	if s.rbacLoader == nil {
		// Accept known runtime roles as fallback
		validRoles := map[string]bool{
			"owner":       true,
			"participant": true,
			"observer":    true,
			"external":    true,
		}
		return validRoles[role]
	}

	// Get all runtime-level roles from RBAC configuration
	validRoles := s.rbacLoader.GetAllRuntimeLevelRoles()
	for _, validRole := range validRoles {
		if validRole == role {
			return true
		}
	}

	return false
}

// isRoleInParties checks if a role is defined in the contract parties array
func isRoleInParties(role string, parties []string) bool {
	for _, party := range parties {
		if party == role {
			return true
		}
	}
	return false
}

// GetContractGraphForThread fetches the contract graph for a given thread
func (s *ThreadService) GetContractGraphForThread(thread *models.Thread) (*models.ContractGraph, error) {
	if thread.ContractName == "" {
		return nil, fmt.Errorf("thread has no contract")
	}

	// Determine version to use (0 means latest)
	version := 0
	if thread.ContractVersion != nil {
		version = *thread.ContractVersion
	}

	return s.contractValidator.GetContractGraph(thread.ContractName, version, thread.CompanyID)
}

// GetContractValidator returns the contract validator service
func (s *ThreadService) GetContractValidator() interfaces.ContractValidator {
	return s.contractValidator
}

// HandleAddRefs adds external references to a thread
func (s *ThreadService) HandleAddRefs(req *models.AddRefsRequest, ownerID string) *models.AddRefsResponse {
	// Validate request
	if req.ThreadID == "" {
		return &models.AddRefsResponse{
			Action:  "addRefs",
			Status:  "error",
			Message: "Thread ID is required",
		}
	}

	if len(req.Refs) == 0 {
		return &models.AddRefsResponse{
			Action:  "addRefs",
			Status:  "error",
			Message: "At least one ref is required",
		}
	}

	// Verify thread exists and user has access
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	thread, err := s.repo.Get(ctx, req.ThreadID)
	if err != nil {
		return &models.AddRefsResponse{
			Action:  "addRefs",
			Status:  "error",
			Message: "Thread not found",
		}
	}

	// Verify user has write permission for the thread
	// This checks both ownership and explicit write permissions
	hasWriteAccess, err := s.accessService.CheckThreadAccess(req.ThreadID, ownerID, "write", thread)
	if err != nil {
		return &models.AddRefsResponse{
			Action:  "addRefs",
			Status:  "error",
			Message: "Failed to verify permissions",
		}
	}

	if !hasWriteAccess {
		return &models.AddRefsResponse{
			Action:  "addRefs",
			Status:  "error",
			Message: "Access denied: write permission required",
		}
	}

	// Store refs atomically
	refsCtx, refsCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer refsCancel()
	if err := s.repo.AddRefs(refsCtx, req.ThreadID, req.Refs); err != nil {
		return &models.AddRefsResponse{
			Action:  "addRefs",
			Status:  "error",
			Message: fmt.Sprintf("Failed to store refs: %v", err),
		}
	}

	// Publish refs as individual events to NATS for archival
	if s.natsArchivalPublisher != nil {
		go func() {
			pubCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			for key, value := range req.Refs {
				refEvent := map[string]interface{}{
					"threadId": req.ThreadID,
					"refKey":   key,
					"refValue": value,
					"action":   "ref_added",
				}
				if err := s.natsArchivalPublisher.PublishThreadMetadata(pubCtx, refEvent); err != nil {
					fmt.Printf("❌ ERROR: Failed to publish ref %s to NATS: %v\n", key, err)
				} else {
					fmt.Printf("✅ [NATS-ARCHIVAL] Published ref %s for thread %s\n", key, req.ThreadID)
				}
			}
		}()
	}

	return &models.AddRefsResponse{
		Action:   "addRefs",
		Status:   "success",
		Message:  fmt.Sprintf("Added %d refs to thread", len(req.Refs)),
		ThreadID: req.ThreadID,
	}
}

// recordThreadCreationActivity records thread creation activity asynchronously
func (s *ThreadService) recordThreadCreationActivity(threadID, ownerID string, access *interfaces.UserAccess, runtimeRole string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, exists := s.connectionMgr.GetClient(ownerID)
	serviceName := ""
	if exists {
		serviceName = client.ServiceName
	}

	if err := s.activityRepo.RecordAccessGranted(ctx, threadID, ownerID, access, "self", serviceName, runtimeRole); err != nil {
		log.Printf("Failed to record access granted activity: %v", err)
	}
}

// publishThreadMetadataAsync publishes thread metadata and activity logs asynchronously
func (s *ThreadService) publishThreadMetadataAsync(threadID, ownerID, companyID string, thread *models.Thread, role string) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("❌ PANIC in thread metadata goroutine: %v\n", r)
		}
	}()

	fmt.Printf("🔄 DEBUG: Starting thread metadata goroutine for thread %s\n", threadID)

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

	fmt.Printf("🔄 DEBUG: About to write to streams:thread_metadata for thread %s\n", threadID)

	// Write to thread_metadata stream for normalized table
	streamValues := map[string]interface{}{
		"threadId":        threadID,
		"ownerId":         ownerID,
		"companyId":       companyID,
		"contractId":      contractID,
		"contractName":    thread.ContractName,
		"contractVersion": contractVersion,
		"error":           "",
		"startedAt":       thread.StartedAt.Format(time.RFC3339),
		"maxlen":          "~",
		"limit":           100000,
	}

	// Publish to NATS for archival
	if s.natsArchivalPublisher != nil {
		pubCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := s.natsArchivalPublisher.PublishThreadMetadata(pubCtx, streamValues); err != nil {
			fmt.Printf("❌ ERROR: Failed to publish thread metadata to NATS: %v\n", err)
		} else {
			fmt.Printf("✅ SUCCESS: Thread metadata published to NATS for thread %s\n", threadID)
		}

		// Publish refs as individual events
		if thread.Refs != nil && len(thread.Refs) > 0 {
			for key, value := range thread.Refs {
				refEvent := map[string]interface{}{
					"threadId": threadID,
					"refKey":   key,
					"refValue": value,
					"action":   "ref_added",
				}
				if err := s.natsArchivalPublisher.PublishThreadMetadata(pubCtx, refEvent); err != nil {
					fmt.Printf("❌ ERROR: Failed to publish ref %s to NATS: %v\n", key, err)
				}
			}
			fmt.Printf("✅ SUCCESS: Published %d refs to NATS for thread %s\n", len(thread.Refs), threadID)
		}
	}

	// Write thread_created event to activity log
	client, exists := s.connectionMgr.GetClient(ownerID)
	serviceName := ""
	if exists {
		serviceName = client.ServiceName
	}

	activityValues := map[string]interface{}{
		"type":             "thread_created",
		"thread_id":        threadID,
		"owner_id":         ownerID,
		"actor":            ownerID,
		"actor_service":    serviceName,
		"contract_id":      contractID,
		"contract_name":    thread.ContractName,
		"contract_version": contractVersion,
		"role":             role,
		"timestamp":        thread.StartedAt.Format(time.RFC3339),
	}

	// Publish to NATS for archival
	if s.natsArchivalPublisher != nil {
		pubCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.natsArchivalPublisher.PublishActivityLog(pubCtx, activityValues); err != nil {
			fmt.Printf("❌ ERROR: Failed to publish activity log to NATS: %v\n", err)
		}
	}
}
