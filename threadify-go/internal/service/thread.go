package service

import (
	"context"
	"fmt"
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
}

func NewThreadService(repo interfaces.ThreadRepository, graphRepo interfaces.ContractGraphRepository, stepEventService interfaces.StepEventProcessor, cacheManager interfaces.CacheManager, connectionMgr interfaces.ConnectionManager, contractValidator interfaces.ContractValidator) *ThreadService {
	return &ThreadService{
		repo:              repo,
		graphRepo:         graphRepo,
		stepEventService:  stepEventService,
		cacheManager:      cacheManager,
		connectionMgr:     connectionMgr,
		contractValidator: contractValidator,
	}
}

// NewThreadServiceWithDefaults creates ThreadService with concrete implementations (for production)
func NewThreadServiceWithDefaults(db *database.PostgresDB, valkeyService *database.ValkeyService, stepEventService *StepEventService, contractTTLSeconds, threadTTLSeconds int) *ThreadService {
	// Create cache service first
	cacheService := NewCacheService()

	// Create repositories
	contractRepo := postgres.NewContractRepository(db.Pool)
	valkeyGraphRepo := valkey.NewContractGraphRepository(valkeyService, contractTTLSeconds) // Configurable TTL for graphs

	return NewThreadService(
		valkey.NewThreadRepository(valkeyService, threadTTLSeconds), // Configurable TTL
		valkeyGraphRepo, // Valkey contract graph repository
		stepEventService,
		cacheService,           // In-memory cache service
		NewConnectionService(), // In-memory connection service
		NewContractValidationService(valkeyGraphRepo, contractRepo, cacheService), // Contract validation service with three-tier caching
	)
}

func (s *ThreadService) HandleConnect(req *models.ConnectRequest) *models.ConnectResponse {
	if req.ApiKey == "" {
		return &models.ConnectResponse{
			Action:  "connect",
			Status:  "error",
			Message: "API key is required",
		}
	}

	if req.OwnerID == "" {
		return &models.ConnectResponse{
			Action:  "connect",
			Status:  "error",
			Message: "Owner ID is required",
		}
	}

	// Use connection manager to handle connection
	err := s.connectionMgr.Connect(req.OwnerID, req.ApiKey, req.ServiceName)
	if err != nil {
		return &models.ConnectResponse{
			Action:  "connect",
			Status:  "error",
			Message: fmt.Sprintf("Failed to connect: %v", err),
		}
	}

	return &models.ConnectResponse{
		Action:           "connect",
		Status:           "success",
		Message:          "Connected successfully",
		OwnerID:          req.OwnerID,
		SubscribedEvents: req.SubscribedEvents,
	}
}

func (s *ThreadService) HandleStartThread(req *models.StartThreadRequest, ownerID string) *models.StartThreadResponse {
	if ownerID == "" || !s.connectionMgr.IsConnected(ownerID) {
		return &models.StartThreadResponse{
			Action:  "startThread",
			Status:  "error",
			Message: "Not authenticated. Please connect first.",
		}
	}

	contractID := ""
	contractVersion := 0

	threadID := uuid.New().String()

	// Parse contract identifier if provided
	if req.ContractID != "" {
		contractID, contractVersion = parseContractIdentifier(req.ContractID)

		// Load contract graph into cache using the contract validator
		if err := s.contractValidator.LoadContractGraphIntoCache(contractID, contractVersion); err != nil {
			return &models.StartThreadResponse{
				Action:  "startThread",
				Status:  "error",
				Message: fmt.Sprintf("Failed to load contract: %v", err),
			}
		}
	}

	// Create and persist the Thread object
	var thread *models.Thread
	if req.ContractID != "" {
		thread = models.NewThread(threadID, contractID, contractVersion, ownerID)
	} else {
		thread = models.NewThread(threadID, "", 0, ownerID)
	}

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

	return &models.StartThreadResponse{
		Action:     "startThread",
		Status:     "success",
		Message:    "Thread started successfully",
		ThreadID:   threadID,
		ContractID: req.ContractID,
	}
}

func (s *ThreadService) HandleRecordEvent(req *models.RecordEventRequest, ownerID string) *models.RecordEventResponse {
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

	// Optional: Validate step name exists in contract (if thread has contract)
	thread, err := s.getThread(req.ThreadID)
	if err != nil {
		return &models.RecordEventResponse{
			Action:  "recordThreadEvent",
			Status:  "error",
			Message: fmt.Sprintf("Thread not found: %s", req.ThreadID),
		}
	}

	// Thread exists - validate contract if needed
	if thread.ContractID != nil && *thread.ContractID != "" {
		contractVersion := 1 // Default fallback
		if thread.ContractVersion != nil {
			contractVersion = *thread.ContractVersion
		}

		if validationErr := s.contractValidator.ValidateStepInContract(*thread.ContractID, contractVersion, req.StepName, req.Context); validationErr != nil {
			return &models.RecordEventResponse{
				Action:  "recordThreadEvent",
				Status:  "error",
				Message: fmt.Sprintf("Contract validation failed: %v", validationErr),
			}
		}
	}

	// Check if step already completed and prevent duplicate updates
	if thread.Steps != nil {
		if existingStep, exists := thread.Steps[req.StepName]; exists && existingStep.IsCompleted {
			return &models.RecordEventResponse{
				Action:  "recordThreadEvent",
				Status:  "error",
				Message: fmt.Sprintf("Step '%s' is already completed and cannot be updated", req.StepName),
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

	stepEvent := models.StepEvent{
		StepID:      stepID,
		Type:        req.Type,         // Use dynamic type from request
		Context:     contextInterface, // Keep original context separate and intact
		Status:      req.Status,
		Timestamp:   time.Now().UTC(), // Use UTC with nanosecond precision for hash uniqueness
		ServiceName: serviceName,
		ThreadID:    req.ThreadID,
		StepName:    req.StepName,
		StartedAt:   req.StartedAt,
		FinishedAt:  req.FinishedAt,
	}

	// Queue step event for async cryptographic processing
	if err := s.stepEventService.ProcessStepEvent(stepEvent); err != nil {
		return &models.RecordEventResponse{
			Action:  "recordThreadEvent",
			Status:  "error",
			Message: fmt.Sprintf("Failed to queue step event: %v", err),
		}
	}

	// Update thread step state
	now := time.Now()
	stepCompleted := req.Status == "completed" || req.Status == "success"

	newStepState := &models.StepState{
		ID:          req.StepName, // Maps to stepName from StepEvent
		Status:      req.Status,   // "completed" | "failed" | "pending"
		CreatedAt:   now,
		UpdatedAt:   now,
		RetryCount:  0, // Will be incremented on retries
		IsCompleted: stepCompleted,
	}

	// If this is a retry, increment retry count
	if existingStep, exists := thread.Steps[req.StepName]; exists {
		newStepState.RetryCount = existingStep.RetryCount + 1
		newStepState.CreatedAt = existingStep.CreatedAt // Keep original creation time
	}

	// Update thread's step state
	if thread.Steps == nil {
		thread.Steps = make(map[string]*models.StepState)
	}
	thread.Steps[req.StepName] = newStepState

	// Update thread in Valkey with new step state
	if err := s.repo.Save(context.Background(), thread); err != nil {
		// Log error but don't fail the response since step event is already queued
		// TODO: Add proper logging here
		fmt.Printf("Warning: Failed to update thread step state: %v\n", err)
	}

	return &models.RecordEventResponse{
		Action:   "recordThreadEvent",
		Status:   "success",
		Message:  "Step Event recorded successfully",
		ThreadID: req.ThreadID,
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
