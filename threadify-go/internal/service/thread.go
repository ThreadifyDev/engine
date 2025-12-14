package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/threadify/engine/internal/database"
	apperrors "github.com/threadify/engine/internal/errors"
	"github.com/threadify/engine/internal/models"
	"github.com/threadify/engine/internal/repository/postgres"
	"github.com/threadify/engine/internal/repository/valkey"
)

type ThreadService struct {
	pgRepo       *postgres.ThreadRepository
	valkeyRepo   *valkey.ThreadRepository
	contractRepo *postgres.ContractRepository
	graphRepo    *valkey.ContractGraphRepository
	clients      map[string]*models.ConnectedClient // In-memory client tracking
}

func NewThreadService(db *database.PostgresDB, valkeyService *database.ValkeyService) *ThreadService {
	return &ThreadService{
		pgRepo:       postgres.NewThreadRepository(db.Pool),
		valkeyRepo:   valkey.NewThreadRepository(valkeyService, 3600), // 1 hour TTL
		contractRepo: postgres.NewContractRepository(db.Pool),
		graphRepo:    valkey.NewContractGraphRepository(valkeyService, 7200), // 2 hour TTL for graphs
		clients:      make(map[string]*models.ConnectedClient),
	}
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

	client := &models.ConnectedClient{
		OwnerID:          req.OwnerID,
		ApiKey:           req.ApiKey,
		ConnectedAt:      time.Now(),
		SubscribedEvents: req.SubscribedEvents,
	}

	// Store client in memory
	s.clients[req.OwnerID] = client

	return &models.ConnectResponse{
		Action:           "connect",
		Status:           "success",
		Message:          "Connected successfully",
		OwnerID:          req.OwnerID,
		SubscribedEvents: req.SubscribedEvents,
	}
}

func (s *ThreadService) HandleStartThread(req *models.StartThreadRequest, ownerID string) *models.StartThreadResponse {
	if ownerID == "" {
		return &models.StartThreadResponse{
			Action:  "startThread",
			Status:  "error",
			Message: "Not authenticated. Please connect first.",
		}
	}

	threadID := uuid.New().String()

	// Validate contract exists and load graph into Valkey
	if req.ContractID != "" {
		if err := s.loadContractGraph(context.Background(), req.ContractID); err != nil {
			return &models.StartThreadResponse{
				Action:  "startThread",
				Status:  "error",
				Message: fmt.Sprintf("Failed to load contract: %v", err),
			}
		}
	}

	return &models.StartThreadResponse{
		Action:     "startThread",
		Status:     "success",
		Message:    "Thread started successfully",
		ThreadID:   threadID,
		ContractID: req.ContractID,
	}
}

func (s *ThreadService) HandleRecordEvent(req *models.RecordEventRequest, ownerID string) *models.RecordEventResponse {
	if ownerID == "" {
		return &models.RecordEventResponse{
			Action:  "recordThreadEvent",
			Status:  "error",
			Message: "Not authenticated. Please connect first.",
		}
	}

	if req.ThreadID == "" {
		return &models.RecordEventResponse{
			Action:  "recordThreadEvent",
			Status:  "error",
			Message: "Thread ID is required",
		}
	}

	return &models.RecordEventResponse{
		Action:   "recordThreadEvent",
		Status:   "success",
		Message:  "Event recorded successfully",
		ThreadID: req.ThreadID,
	}
}

func (s *ThreadService) HandleClose(ownerID string) *models.CloseConnectionResponse {
	if ownerID != "" {
		delete(s.clients, ownerID)
	}

	return &models.CloseConnectionResponse{
		Action:  "closeConnection",
		Status:  "success",
		Message: "Connection closed successfully",
	}
}

// loadContractGraph validates that a contract exists and loads its graph into Valkey cache
// contractNameOrID can be:
//   - contract name: "product_delivery"
//   - contract name with version: "product_delivery:2"
//   - contract UUID: "uuid-string"
func (s *ThreadService) loadContractGraph(ctx context.Context, contractNameOrID string) error {
	// Parse contract name and optional version
	contractName, requestedVersion := parseContractIdentifier(contractNameOrID)

	// DEBUG: Log what we're looking for
	fmt.Printf("[DEBUG] Looking for contract: '%s' (version: %d)\n", contractName, requestedVersion)

	// 1. Validate contract exists - try by name first, then by ID
	contract, err := s.contractRepo.GetByNameSlim(ctx, contractName)
	if err != nil {
		fmt.Printf("[DEBUG] GetByNameSlim failed: %v\n", err)
		// If not found by name, try by ID (for backward compatibility)
		contract, err = s.contractRepo.GetByID(ctx, contractName)
		if err != nil {
			fmt.Printf("[DEBUG] GetByID also failed: %v\n", err)
			// TODO: Add structured logging here
			// log.Error("Failed to get contract", "contractNameOrID", contractNameOrID, "error", err)
			return apperrors.NewNotFoundError(apperrors.MsgContractNotFound, err)
		}
	}

	fmt.Printf("[DEBUG] Found contract: ID=%s, Name=%s, LatestVersion=%d\n", contract.ID, contract.Name, contract.LatestVersion)

	// 2. Get the contract version - either specific version or latest
	var contractVersion *models.ContractVersion
	if requestedVersion > 0 {
		// Get specific version
		contractVersion, err = s.contractRepo.GetVersion(ctx, contract.ID, requestedVersion)
		if err != nil {
			// TODO: Add structured logging here
			return apperrors.NewNotFoundError(apperrors.MsgContractVersionNotFound, err)
		}
	} else {
		// Get latest version
		contractVersion, err = s.contractRepo.GetLatestVersion(ctx, contract.ID)
		if err != nil {
			// TODO: Add structured logging here
			return apperrors.NewNotFoundError(apperrors.MsgContractVersionNotFound, err)
		}
	}

	// 3. Validate that the contract version has graph data
	if contractVersion.Graph == nil || len(contractVersion.Graph) == 0 {
		return apperrors.NewValidationError(apperrors.MsgContractIncomplete, nil)
	}

	// 4. Deserialize the graph from JSON
	var contractGraph models.ContractGraph
	if err := json.Unmarshal(contractVersion.Graph, &contractGraph); err != nil {
		// TODO: Add structured logging here
		return apperrors.NewValidationError(apperrors.MsgContractInvalid, err)
	}

	// 5. Save the graph to Valkey cache (metadata passed separately)
	if err := s.graphRepo.Save(ctx, contract.ID, contractVersion.Version, &contractGraph); err != nil {
		// TODO: Add structured logging here
		return apperrors.NewInternalError(apperrors.MsgServiceUnavailable, err)
	}

	return nil
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
