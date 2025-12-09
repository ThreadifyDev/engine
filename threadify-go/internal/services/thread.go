package services

import (
	"time"

	"github.com/google/uuid"
	"github.com/threadify/engine/internal/models"
)

// ClientQueue interface for dependency injection and testing
type ClientQueue interface {
	AddClient(client *models.ConnectedClient) error
	GetClient(ownerID string) (*models.ConnectedClient, error)
	RemoveClient(ownerID string) error
}

type ThreadService struct {
	queue ClientQueue
}

func NewThreadService(queue ClientQueue) *ThreadService {
	return &ThreadService{queue: queue}
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

	if err := s.queue.AddClient(client); err != nil {
		return &models.ConnectResponse{
			Action:  "connect",
			Status:  "error",
			Message: "Failed to register client",
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
	if ownerID == "" {
		return &models.StartThreadResponse{
			Action:  "startThread",
			Status:  "error",
			Message: "Not authenticated. Please connect first.",
		}
	}

	threadID := uuid.New().String()

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
		s.queue.RemoveClient(ownerID)
	}

	return &models.CloseConnectionResponse{
		Action:  "closeConnection",
		Status:  "success",
		Message: "Connection closed successfully",
	}
}
