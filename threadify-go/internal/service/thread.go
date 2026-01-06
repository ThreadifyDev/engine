package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/threadify/engine/internal/config"
	"github.com/threadify/engine/internal/database"
	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/models"
	natsrepo "github.com/threadify/engine/internal/repository/nats"
	"github.com/threadify/engine/internal/repository/postgres"
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
	luaScripts            *LuaScriptManager
	natsArchivalPublisher *natsrepo.ArchivalPublisher
}

func NewThreadService(repo interfaces.ThreadRepository, accessRepo interfaces.AccessRepository, activityRepo interfaces.ActivityRepository, graphRepo interfaces.ContractGraphRepository, stepEventService interfaces.StepEventProcessor, cacheManager interfaces.CacheManager, connectionMgr interfaces.ConnectionManager, contractValidator interfaces.ContractValidator, accessService *ThreadAccessService, invitationService *InvitationTokenService, valkeyClient interfaces.ValkeyClient, luaScripts *LuaScriptManager, stepStateRepo interfaces.StepStateRepository, natsPublisher NotificationPublisher, natsArchivalPublisher *natsrepo.ArchivalPublisher) *ThreadService {
	// Create validation and notification services
	validationService := NewValidationService(valkeyClient)
	notificationService := NewNotificationService(validationService, activityRepo, stepStateRepo, cacheManager, natsPublisher)

	return &ThreadService{
		repo:                  repo,
		accessRepo:            accessRepo,
		activityRepo:          activityRepo,
		graphRepo:             graphRepo,
		stepEventService:      stepEventService,
		cacheManager:          cacheManager,
		connectionMgr:         connectionMgr,
		contractValidator:     contractValidator,
		authService:           NewAuthService("demo-secret", "threadify", "threadify-api", 24),
		accessService:         accessService,
		validationService:     validationService,
		notificationService:   notificationService,
		invitationService:     invitationService,
		valkeyClient:          valkeyClient,
		luaScripts:            luaScripts,
		natsArchivalPublisher: natsArchivalPublisher,
	}
}

// NewThreadServiceWithDefaults creates ThreadService with concrete implementations (for production)
func NewThreadServiceWithDefaults(cfg *config.Config, db *database.PostgresDB, valkeyService *database.ValkeyService, stepEventService *StepEventService, contractTTLSeconds, threadTTLSeconds int) *ThreadService {
	// Create cache service first
	cacheService := NewCacheService()

	// Create repositories
	contractRepo := postgres.NewContractRepository(db.Pool)
	valkeyGraphRepo := valkey.NewContractGraphRepository(valkeyService, contractTTLSeconds) // Configurable TTL for graphs
	threadRepo := valkey.NewThreadRepository(valkeyService, threadTTLSeconds)
	accessRepo := valkey.NewAccessRepository(valkeyService)

	// Create and load Lua scripts
	luaScripts := NewLuaScriptManager(valkeyService)
	if err := luaScripts.LoadScripts(context.Background()); err != nil {
		fmt.Printf("Warning: Failed to load Lua scripts: %v\n", err)
	}

	// Create step state repository and load its scripts
	stepStateRepo := valkey.NewStepStateRepository(valkeyService)
	if err := stepStateRepo.LoadScripts(context.Background()); err != nil {
		fmt.Printf("Warning: Failed to load step state repository scripts: %v\n", err)
	}

	// Create thread access service for permission/role management
	accessService := NewThreadAccessService(accessRepo, cacheService, luaScripts)

	// Create invitation service
	invitationService := NewInvitationTokenService("demo-secret")

	// Create scope resolver for notification access control
	scopeResolver := NewScopeResolver(cfg, valkeyGraphRepo, threadRepo)

	// Initialize NATS publisher and consumer for notifications (graceful degradation if NATS unavailable)
	var natsPublisher NotificationPublisher
	var natsConsumer *NotificationConsumer
	var natsArchivalPublisher *natsrepo.ArchivalPublisher
	natsClient, err := natsrepo.NewClient(&cfg.NATS)
	if err != nil {
		fmt.Printf("Warning: Failed to connect to NATS - notifications and archival will be disabled: %v\n", err)
		natsPublisher = nil
		natsConsumer = nil
		natsArchivalPublisher = nil
	} else {
		natsPublisher = natsrepo.NewPublisher(natsClient, scopeResolver, accessRepo)
		natsConsumer = NewNotificationConsumer(natsClient, scopeResolver)
		natsArchivalPublisher = natsrepo.NewArchivalPublisher(natsClient)
		fmt.Printf("NATS publisher, consumer, and archival publisher initialized successfully\n")
	}

	// Create activity repository with NATS publisher
	activityRepo := valkey.NewActivityRepository(valkeyService, natsArchivalPublisher)

	service := NewThreadService(
		threadRepo,      // Valkey thread repository
		accessRepo,      // Valkey access repository
		activityRepo,    // Valkey activity repository
		valkeyGraphRepo, // Valkey contract graph repository
		stepEventService,
		cacheService,           // In-memory cache service
		NewConnectionService(), // In-memory connection service
		NewContractValidationService(valkeyGraphRepo, contractRepo, cacheService), // Contract validation service with three-tier caching
		accessService,         // Thread access service for permissions/roles
		invitationService,     // Invitation token service
		valkeyService,         // Valkey client for orchestration
		luaScripts,            // Lua script manager for orchestration
		stepStateRepo,         // Step state repository for atomic validations
		natsPublisher,         // NATS publisher for notifications (can be nil)
		natsArchivalPublisher, // NATS archival publisher (can be nil)
	)
	service.valkeyClient = valkeyService
	service.luaScripts = luaScripts
	service.scopeResolver = scopeResolver
	service.notificationConsumer = natsConsumer

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
		// This returns the actual version loaded (resolves version 0 to latest)
		actualVersion, err := s.contractValidator.LoadContractGraphIntoCache(parsedContractName, contractVersion)
		if err != nil {
			return &models.StartThreadResponse{
				Action:  "startThread",
				Status:  "error",
				Message: fmt.Sprintf("Failed to load contract: %v", err),
			}
		}
		contractVersion = actualVersion // Use the actual version that was loaded
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

	// Grant access to thread creator (role + full permissions)
	// invitedBy = "self" indicates this is the thread creator
	creatorRole := req.Role
	if creatorRole == "" {
		creatorRole = "owner" // Default role if not specified
	}
	creatorPermissions := []string{"read", "write", "invite", "manage"}

	// Use GrantOrUpdateThreadAccess to ensure scope resolution happens
	// Creator always gets isCreator=true, which resolves to "owner" scope
	if err := s.GrantOrUpdateThreadAccess(threadID, ownerID, creatorRole, creatorPermissions, "self", true, nil); err != nil {
		// Log error but don't fail thread creation
		fmt.Printf("Warning: failed to grant creator access: %v\n", err)
	}

	// Write thread metadata to stream for archival (async, don't fail if it fails)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("❌ PANIC in thread metadata goroutine: %v\n", r)
			}
		}()

		fmt.Printf("🔄 DEBUG: Starting thread metadata goroutine for thread %s\n", threadID)
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

		fmt.Printf("🔄 DEBUG: About to write to streams:thread_metadata for thread %s\n", threadID)

		// Write to thread_metadata stream for normalized table
		streamValues := map[string]interface{}{
			"threadId":        threadID, // Use threadId to avoid collision with Redis stream ID
			"ownerId":         ownerID,
			"companyId":       companyID, // Add company_id to satisfy foreign key constraint
			"contractId":      contractID,
			"contractVersion": contractVersion,
			"error":           "", // Initialize with empty error
			"startedAt":       thread.StartedAt.Format(time.RFC3339),
			"maxlen":          "~",
			"limit":           100000,
		}

		// Publish to NATS for archival (async)
		if s.natsArchivalPublisher != nil {
			go func() {
				pubCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := s.natsArchivalPublisher.PublishThreadMetadata(pubCtx, streamValues); err != nil {
					fmt.Printf("❌ ERROR: Failed to publish thread metadata to NATS: %v\n", err)
				} else {
					fmt.Printf("✅ SUCCESS: Thread metadata published to NATS for thread %s\n", threadID)
				}
			}()
		}

		// Write thread_created event to activity log
		// Get service name from connection manager
		client, exists := s.connectionMgr.GetClient(ownerID)
		serviceName := ""
		if exists {
			serviceName = client.ServiceName
		}

		activityValues := map[string]interface{}{
			"type":             "thread_created",
			"thread_id":        threadID,
			"owner_id":         ownerID,
			"actor":            ownerID,     // user-123
			"actor_service":    serviceName, // merchant-service
			"contract_id":      contractID,
			"contract_name":    thread.ContractName,
			"contract_version": contractVersion,
			"role":             req.Role,
			"timestamp":        thread.StartedAt.Format(time.RFC3339),
		}

		// 1. Write to per-thread LIST for fast queries
		activityList := fmt.Sprintf("thread:%s:activity", threadID)
		eventJSON, _ := json.Marshal(activityValues)
		s.valkeyClient.LPush(ctx, activityList, string(eventJSON))
		s.valkeyClient.Expire(ctx, activityList, 7*24*time.Hour)

		// 2. Publish to NATS for archival (async)
		if s.natsArchivalPublisher != nil {
			go func() {
				pubCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := s.natsArchivalPublisher.PublishActivityLog(pubCtx, activityValues); err != nil {
					fmt.Printf("❌ ERROR: Failed to publish activity log to NATS: %v\n", err)
				}
			}()
		}
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

	// Store refs atomically before step processing
	if req.Refs != nil {
		if err := s.repo.AddRefs(context.Background(), req.ThreadID, req.Refs); err != nil {
			return &models.RecordEventResponse{
				Action:  "recordThreadEvent",
				Status:  "error",
				Message: fmt.Sprintf("Failed to store refs: %v", err),
			}
		}
	}

	// Process step event immediately
	if err := s.stepEventService.RecordStepEventDirect(*stepEvent, ownerID); err != nil {
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
		return nil, fmt.Errorf("failed to get contract graph: %v", err)
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

	contractID := "contract-123" // This would come from thread data

	// Create JWT token
	threadToken, err := s.invitationService.CreateToken(threadID, contractID, ownerID, req.Role, permissions, expiry)
	if err != nil {
		return nil, fmt.Errorf("failed to create invitation token: %v", err)
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
	var permissions []string
	var thread *models.Thread

	// Mode 1: Token-based join (invitation)
	if req.ThreadToken != "" {
		// Validate JWT token and extract claims
		claims, err := s.invitationService.ValidateToken(req.ThreadToken)
		if err != nil {
			return nil, fmt.Errorf("invalid thread token: %v", err)
		}

		threadID = claims.ThreadID
		role = claims.Role
		permissions = strings.Split(claims.Permissions, ",")
		invitedBy = claims.InvitedBy

		// Mode 2: Direct join (same company, no token)
	} else if req.ThreadID != "" && req.Role != "" {
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
		permissions = []string{"read", "write"} // Default permissions for direct join
		invitedBy = companyID                   // Company ID as inviter

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
	err = s.GrantOrUpdateThreadAccess(threadID, ownerID, role, permissions, invitedBy, false, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to grant access: %v", err)
	}

	return &models.JoinThreadResponse{
		Action:      "joinThread",
		Status:      "success",
		ThreadID:    threadID,
		Role:        role,
		Permissions: strings.Join(permissions, ","),
		Message:     "Successfully joined thread",
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

// GetThread retrieves thread from cache first, then Valkey as fallback
func (s *ThreadService) GetThread(threadID string) (*models.Thread, error) {
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

// GrantOrUpdateThreadAccess grants or updates user access using unified method with proper orchestration
func (s *ThreadService) GrantOrUpdateThreadAccess(threadID, userID, role string, permissions []string, invitedBy string, isCreator bool, explicitScope *string) error {
	// 1. Resolve notification scope
	scope, err := s.scopeResolver.ResolveScope(
		context.Background(),
		threadID,
		userID,
		role,
		isCreator,
		explicitScope,
	)
	if err != nil {
		log.Printf("Failed to resolve scope for user %s in thread %s: %v", userID, threadID, err)
		// Continue with empty scope rather than failing the entire operation
		scope = ""
	}

	// 2. Grant access via AccessRepository (atomic via Lua script)
	access, err := s.accessRepo.GrantOrUpdateAccess(context.Background(), threadID, userID, role, permissions, invitedBy, s.luaScripts)
	if err != nil {
		return fmt.Errorf("failed to grant access: %w", err)
	}

	// 3. Record activity via ActivityRepository (async, don't block main operation)
	go func() {
		ctx := context.Background()
		// Get service name from connection manager
		client, exists := s.connectionMgr.GetClient(userID)
		serviceName := ""
		if exists {
			serviceName = client.ServiceName
		}
		if err := s.activityRepo.RecordAccessGranted(ctx, threadID, userID, access, invitedBy, serviceName, scope); err != nil {
			log.Printf("Failed to record access granted activity: %v", err)
		}
	}()

	return nil
}

// IsValidRole validates if a role name is valid
func (s *ThreadService) IsValidRole(role string) bool {
	// Add your role validation logic here
	// For now, just check it's not empty
	return true
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

	return s.contractValidator.GetContractGraph(thread.ContractName, version)
}

// AssignThreadRole assigns a role to a user in a thread (deprecated - use GrantOrUpdateThreadAccess)
func (s *ThreadService) AssignThreadRole(threadID, role, userID string) error {
	return s.accessService.AssignRole(threadID, role, userID)
}

// SetThreadPermissions sets permissions for a user in a thread (deprecated - use GrantOrUpdateThreadAccess)
func (s *ThreadService) SetThreadPermissions(threadID, userID string, permissions []string) error {
	return s.accessService.SetUserPermissions(threadID, userID, permissions)
}
