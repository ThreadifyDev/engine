package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"threadify-go/shared/rbac"

	shderrors "threadify-go/shared/errors"

	"github.com/google/uuid"
	"github.com/threadify/engine/internal/config"
	"github.com/threadify/engine/internal/database"
	"github.com/threadify/engine/internal/domain"
	"github.com/threadify/engine/internal/metrics"
	"github.com/threadify/engine/internal/perf"
	natsrepo "github.com/threadify/engine/internal/repository/nats"
	"github.com/threadify/engine/internal/repository/valkey"
	"github.com/threadify/engine/internal/utils"
	"github.com/threadify/engine/internal/workerpool"
	"go.uber.org/zap"
)

// fallbackValidRoles is used when the RBAC loader is unavailable.
var fallbackValidRoles = map[string]bool{
	"owner":       true,
	"participant": true,
	"observer":    true,
	"external":    true,
}

// ThreadService orchestrates thread operations across multiple repositories.
type ThreadService struct {
	repo                  domain.ThreadRepository
	accessRepo            domain.AccessRepository
	activityRepo          domain.ActivityRepository
	graphRepo             domain.ContractGraphRepository
	stepEventService      domain.StepEventProcessor
	cacheManager          domain.CacheManager
	connectionMgr         domain.ConnectionManager
	contractValidator     domain.ContractGraphValidator
	authService           *AuthService
	accessService         *ThreadAccessService
	validationService     *ValidationService
	notificationService   *NotificationService
	invitationService     *InvitationTokenService
	scopeResolver         *ScopeResolver
	notificationConsumer  *NotificationConsumer
	planService           domain.PlanService
	valkeyClient          domain.ThreadValkeyClient
	luaScripts            *valkey.LuaScriptManager
	natsArchivalPublisher *natsrepo.ArchivalPublisher
	rbacLoader            *rbac.Loader
	writeBackPool         *workerpool.Pool
	logger                *zap.Logger
}

// NewThreadService creates a ThreadService using the builder pattern.
// natsPublisher and natsArchivalPublisher may be nil for graceful degradation.
// workerPools may be nil in tests (unbounded goroutines will be used).
func NewThreadService(
	cfg *config.Config,
	db *database.PostgresDB,
	valkeyService *database.ValkeyService,
	stepEventService *StepEventService,
	threadRepo *valkey.ThreadRepository,
	contractTTLSeconds int,
	natsClient *natsrepo.Client,
	natsPublisher domain.NotificationPublisher,
	natsArchivalPublisher *natsrepo.ArchivalPublisher,
	authService *AuthService,
	planService domain.PlanService,
	cacheManager domain.CacheManager,
	workerPools *workerpool.Pools,
	logger *zap.Logger,
) *ThreadService {
	svc, err := NewThreadServiceBuilder().
		WithConfig(cfg).
		WithDatabase(db).
		WithValkey(valkeyService).
		WithStepEventService(stepEventService).
		WithThreadRepository(threadRepo).
		WithContractTTL(contractTTLSeconds).
		WithNATSClient(natsClient).
		WithNATSPublisher(natsPublisher).
		WithNATSArchivalPublisher(natsArchivalPublisher).
		WithAuthService(authService).
		WithPlanService(planService).
		WithCacheManager(cacheManager).
		WithWorkerPools(workerPools).
		WithLogger(logger).
		Build()
	if err != nil {
		logger.Fatal("failed to build ThreadService", zap.Error(err))
	}
	return svc
}

func (s *ThreadService) GetNotificationConsumer() domain.NotificationConsumer {
	return s.notificationConsumer
}

func (s *ThreadService) HandleConnect(ctx context.Context, req *domain.ConnectCmd) *domain.ConnectResponse {
	if req.ApiKey == "" {
		return &domain.ConnectResponse{Action: ActionConnect, Status: StepStatusError, Message: "API key is required"}
	}

	userInfo, err := s.authService.ValidateApiKey(req.ApiKey)
	if err != nil {
		return &domain.ConnectResponse{Action: ActionConnect, Status: StepStatusError, Message: "authentication failed"}
	}

	meter, err := s.planService.CheckBalancePositive(ctx, userInfo.CompanyID)
	if err != nil {
		switch {
		case errors.Is(err, ErrNoAccount):
			return &domain.ConnectResponse{
				Action:  ActionConnect,
				Status:  StepStatusError,
				Message: "No billing account found. Please set up a credit account in the dashboard.",
			}
		case errors.Is(err, ErrInsufficientCredit):
			return &domain.ConnectResponse{
				Action:  ActionConnect,
				Status:  StepStatusError,
				Message: "Insufficient credits. Please top up your account to continue.",
			}
		default:
			s.logger.Error("failed to verify limits during connect", zap.Error(err), zap.String("company_id", userInfo.CompanyID))
			return &domain.ConnectResponse{
				Action:  ActionConnect,
				Status:  StepStatusError,
				Message: "Failed to verify credit account status. Please try again later.",
			}
		}
	}

	if meter == nil {
		return &domain.ConnectResponse{
			Action:  ActionConnect,
			Status:  StepStatusError,
			Message: "Credit account details are currently unavailable. Please contact support.",
		}
	}

	if err := s.connectionMgr.ConnectWithOwnerAndCompany(userInfo.OwnerID, req.ApiKey, req.ServiceName, userInfo.CompanyID); err != nil {
		return &domain.ConnectResponse{Action: ActionConnect, Status: StepStatusError, Message: "failed to establish connection"}
	}

	return &domain.ConnectResponse{
		Action:    ActionConnect,
		Status:    StepStatusSuccess,
		Message:   "Connected successfully",
		OwnerID:   userInfo.OwnerID,
		CompanyID: userInfo.CompanyID,
	}
}

func (s *ThreadService) HandleStartThread(ctx context.Context, req *domain.StartThreadCmd, ownerID, companyID string) *domain.StartThreadResponse {
	start := perf.Now()
	perf.LogStructured("HandleStartThread BEGIN", zap.String("owner", ownerID), zap.String("contract", req.ContractName))

	errResp := func(msg string) *domain.StartThreadResponse {
		return &domain.StartThreadResponse{Action: ActionStartThread, Status: StepStatusError, Message: msg}
	}

	if ownerID == "" || !s.connectionMgr.IsConnected(ownerID) {
		return errResp("Not authenticated. Please connect first.")
	}
	if req.ContractName != "" && req.Role == "" {
		return errResp("Role is required when contract name is provided")
	}

	if err := s.planService.DecrementIngress(ctx, companyID, 1); err != nil {
		return errResp("Cannot start thread: " + err.Error())
	}

	var contractVersion int
	var parsedContractName, contractUUID string
	var contractGraph *domain.ContractGraph

	if req.ContractName != "" {
		parsedContractName, contractVersion = parseContractIdentifier(req.ContractName)

		t := time.Now()
		actualVersion, err := s.contractValidator.LoadContractGraphIntoCache(parsedContractName, contractVersion, companyID)
		metrics.OperationDuration.WithLabelValues(ActionStartThread, "contract_load").Observe(time.Since(t).Seconds())
		if err != nil {
			return errResp("Failed to load contract")
		}
		contractVersion = actualVersion

		// Get contract graph (already cached from LoadContractGraphIntoCache above)
		contractGraph, err = s.contractValidator.GetContractGraph(parsedContractName, contractVersion, companyID)
		if err != nil {
			return errResp("Failed to load contract graph")
		}

		// Validate role against contract parties
		if len(contractGraph.Parties) > 0 && !slices.Contains(contractGraph.Parties, req.Role) {
			return errResp("Role '" + req.Role + "' is not defined in contract parties")
		}

		t = time.Now()
		contract, err := s.contractValidator.GetContractByNameAndCompany(parsedContractName, companyID)
		metrics.OperationDuration.WithLabelValues(ActionStartThread, "contract_fetch").Observe(time.Since(t).Seconds())
		if err != nil {
			return errResp("Failed to retrieve contract")
		}
		contractUUID = contract.ID
	}

	threadID := uuid.New().String()

	label := StartThreadLabel(req)

	var contractIDPtr *string
	if contractUUID != "" {
		contractIDPtr = &contractUUID
	}
	var contractVersionPtr *int
	if parsedContractName != "" && contractVersion > 0 {
		contractVersionPtr = &contractVersion
	}

	thread := &domain.Thread{
		ID:              threadID,
		Label:           label,
		ContractID:      contractIDPtr,
		ContractName:    parsedContractName,
		ContractVersion: contractVersionPtr,
		OwnerID:         ownerID,
		CompanyID:       companyID,
		Status:          domain.ThreadStatusActive,
		StartedAt:       time.Now(),
	}

	creatorRole := req.Role
	if creatorRole == "" {
		creatorRole = "owner"
	}

	createCtx, createCancel := context.WithTimeout(ctx, 10*time.Second)
	defer createCancel()

	// Thread creator always gets "owner" runtime_role (invariant)
	runtimeRole := "owner"

	// Serialize thread to JSON using explicit mapping for storage
	threadDataBytes, err := json.Marshal(map[string]interface{}{
		"id":              thread.ID,
		"contractId":      thread.ContractID,
		"contractVersion": thread.ContractVersion,
		"contractName":    thread.ContractName,
		"refs":            thread.Refs,
		"ownerId":         thread.OwnerID,
		"companyId":       thread.CompanyID,
		"label":           thread.Label,
		"createdBy":       thread.CreatedBy,
		"status":          string(thread.Status),
		"lastHash":        thread.LastHash,
		"startedAt":       thread.StartedAt,
		"completedAt":     thread.CompletedAt,
		"error":           thread.Error,
	})
	if err != nil {
		return errResp("Failed to serialize thread")
	}
	threadDataStr := string(threadDataBytes)
	threadTTLSeconds := 18000 // 5 hours

	t := time.Now()
	access, err := s.accessService.GrantAccessWithThreadCreation(
		createCtx, threadID, ownerID, creatorRole, runtimeRole, &threadDataStr, &threadTTLSeconds,
	)
	metrics.OperationDuration.WithLabelValues(ActionStartThread, "redis_thread_create").Observe(time.Since(t).Seconds())
	if err != nil {
		return errResp("Failed to create thread")
	}

	s.cacheManager.SetThread(threadID, thread)

	// Consolidate archival publication into a single sequential goroutine to minimize race conditions in the archiver.
	go s.publishThreadInitialArchivalAsync(threadID, ownerID, companyID, thread, req.Role, access, runtimeRole)

	// Process refs in hot cache (Valkey) if provided
	if len(req.Refs) > 0 {
		refsCtx, refsCancel := context.WithTimeout(ctx, 5*time.Second)
		if err := s.repo.AddRefs(refsCtx, threadID, req.Refs); err != nil {
			s.logger.Warn("failed to store refs in cache", zap.String("thread", threadID), zap.Error(err))
		}
		refsCancel()
	}

	// Schedule thread max duration timeout if contract has max_duration validation
	if contractGraph != nil && s.notificationService != nil {
		go func(graph *domain.ContractGraph) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			s.notificationService.scheduleThreadMaxDurationTimeout(ctx, threadID, graph, thread, thread.StartedAt)
		}(contractGraph)
	}

	perf.LogStructured("HandleStartThread COMPLETE", zap.Duration("duration", perf.Since(start)), zap.Bool("success", true))

	return &domain.StartThreadResponse{
		Action:   ActionStartThread,
		Status:   StepStatusSuccess,
		Message:  "Thread started successfully",
		ThreadID: threadID,
	}
}

func StartThreadLabel(req *domain.StartThreadCmd) string {
	if req == nil {
		return ""
	}
	label := req.Label
	if label == "" && req.Refs != nil {
		label = req.Refs["label"]
	}
	return label
}

func (s *ThreadService) HandleRecordEvent(ctx context.Context, req *domain.RecordEventCmd, ownerID, companyID string) *domain.RecordEventResponse {
	start := perf.Now()
	perf.LogStructured("HandleRecordEvent BEGIN",
		zap.String("owner", ownerID),
		zap.String("thread", req.ThreadID),
		zap.String("step", req.StepName),
	)

	errResp := func(msg string) *domain.RecordEventResponse {
		return &domain.RecordEventResponse{Action: ActionRecordThreadEvent, Status: StepStatusError, Message: msg}
	}

	if ownerID == "" || !s.connectionMgr.IsConnected(ownerID) {
		return errResp("Not authenticated. Please connect first.")
	}
	if err := validateRecordEventRequest(req); err != nil {
		return errResp(err.Error())
	}

	account, err := s.planService.GetCurrentLimits(ctx, companyID)
	if err != nil {
		return errResp("Failed to verify subscription: " + err.Error())
	}

	reqBytes, _ := json.Marshal(req)
	if err := s.planService.CheckPayloadSize(ctx, account, int64(len(reqBytes))); err != nil {
		return errResp("PAYLOAD_TOO_LARGE: " + err.Error())
	}

	t := time.Now()
	thread, err := s.getThread(req.ThreadID)
	metrics.OperationDuration.WithLabelValues(ActionRecordThreadEvent, "thread_fetch").Observe(time.Since(t).Seconds())
	if err != nil {
		return errResp("Thread not found: " + req.ThreadID)
	}

	t = time.Now()
	hasAccess, err := s.accessService.CheckThreadAccess(ctx, req.ThreadID, ownerID, "thread.write.*", thread)
	metrics.OperationDuration.WithLabelValues(ActionRecordThreadEvent, "permission_check").Observe(time.Since(t).Seconds())
	if err != nil || !hasAccess {
		return errResp("Access denied: You don't have write permission for this thread")
	}

	if thread.Status == domain.ThreadStatusCompleted {
		return errResp("Cannot add steps to completed thread")
	}

	t = time.Now()
	contentHash := ""
	if len(req.Context) > 0 {
		contentHash = utils.GenerateContextHash(req.Context)
	}
	metrics.OperationDuration.WithLabelValues(ActionRecordThreadEvent, "context_hash").Observe(time.Since(t).Seconds())

	idempotencyKey := req.IdempotencyKey
	if idempotencyKey == "" {
		// Scoped to thread + step + content to prevent broad collisions
		hashInput := fmt.Sprintf("%s:%s:%s", req.ThreadID, req.StepName, contentHash)
		hash := sha256.Sum256([]byte(hashInput))
		idempotencyKey = fmt.Sprintf("sha256:%x", hash)
		s.logger.Debug("auto-generated idempotency key", zap.String("key", idempotencyKey))
	}

	if idempotencyKey != "" {
		idempCtx, idempCancel := context.WithTimeout(ctx, 5*time.Second)
		t = time.Now()
		existingStatus, err := s.repo.GetStepStatus(idempCtx, domain.StepStatusQuery{
			ThreadID:       req.ThreadID,
			StepName:       req.StepName,
			ExpectedStatus: req.Status,
			IdempotencyKey: idempotencyKey,
		}, domain.ThreadReadOptions{WriteBack: true})
		idempCancel()
		metrics.OperationDuration.WithLabelValues(ActionRecordThreadEvent, "idempotency_check").Observe(time.Since(t).Seconds())
		if err == nil && existingStatus != "" {
			if existingStatus == ThreadStatusCompleted || existingStatus == StepStatusSuccess {
				return &domain.RecordEventResponse{
					Action:      ActionRecordThreadEvent,
					Status:      StepStatusError,
					Message:     "Step with this signature already completed",
					IsDuplicate: true,
				}
			}
		}
	}

	if req.IdempotencyKey == "" && idempotencyKey != "" {
		req.IdempotencyKey = idempotencyKey
	}

	var graph *domain.ContractGraph
	var stepNode domain.GraphNode

	if thread.ContractName != "" {
		version := 0
		if thread.ContractVersion != nil {
			version = *thread.ContractVersion
		}

		t = time.Now()
		graph, err = s.contractValidator.GetContractGraph(thread.ContractName, version, thread.CompanyID)
		metrics.OperationDuration.WithLabelValues(ActionRecordThreadEvent, "contract_validate").Observe(time.Since(t).Seconds())
		if err != nil {
			return errResp("failed to load contract")
		}

		var exists bool
		stepNode, exists = graph.Graph.Nodes[req.StepName]
		if !exists {
			return errResp(fmt.Sprintf("Step '%s' not found in contract '%s'", req.StepName, thread.ContractName))
		}

		if !s.hasSuccessfulSteps(thread) {
			if !slices.Contains(graph.Graph.EntryPoints, req.StepName) {
				return errResp(fmt.Sprintf("Thread must start with one of the entry points: %v. Attempted step: '%s'", graph.Graph.EntryPoints, req.StepName))
			}
		}

		if stepNode.Owner != "" {
			if len(graph.Parties) > 0 && !slices.Contains(graph.Parties, stepNode.Owner) {
				return errResp(fmt.Sprintf("Access denied: Step '%s' requires owner role '%s' which is not defined in contract parties: %v", req.StepName, stepNode.Owner, graph.Parties))
			}

			t = time.Now()
			hasRole, err := s.accessService.ValidateUserRoleForStep(ctx, req.ThreadID, ownerID, stepNode.Owner)
			metrics.OperationDuration.WithLabelValues(ActionRecordThreadEvent, "role_validate").Observe(time.Since(t).Seconds())
			if err != nil || !hasRole {
				userRole, _ := s.accessService.GetUserRole(ctx, req.ThreadID, ownerID)
				return errResp(fmt.Sprintf("Access denied: Step '%s' requires owner '%s', you have role '%s'", req.StepName, stepNode.Owner, userRole))
			}
		}

		t = time.Now()
		if err := s.contractValidator.ValidateStepContext(stepNode, req.Context); err != nil {
			metrics.OperationDuration.WithLabelValues(ActionRecordThreadEvent, "step_context_validate").Observe(time.Since(t).Seconds())
			return errResp(fmt.Sprintf("Step validation failed: %v", err))
		}
		metrics.OperationDuration.WithLabelValues(ActionRecordThreadEvent, "step_context_validate").Observe(time.Since(t).Seconds())
	}

	stepID := uuid.New().String()
	serviceName := req.ServiceName
	if serviceName == "" {
		if client, exists := s.connectionMgr.GetClient(ownerID); exists {
			serviceName = client.ServiceName
		}
	}
	if serviceName == "" {
		serviceName = "unknown"
	}

	finishedAtTime, err := time.Parse(time.RFC3339Nano, req.FinishedAt)
	if err != nil {
		s.logger.Warn("failed to parse FinishedAt, falling back to time.Now()",
			zap.String("thread_id", req.ThreadID),
			zap.String("finished_at", req.FinishedAt),
			zap.Error(err),
		)
		finishedAtTime = time.Now()
	}

	contextInterface := make(map[string]interface{}, len(req.Context))
	for k, v := range req.Context {
		contextInterface[k] = v
	}

	stepEvent := domain.StepEvent{
		StepID:         stepID,
		ThreadID:       req.ThreadID,
		StepName:       req.StepName,
		ServiceName:    serviceName,
		Type:           req.Type,
		Status:         req.Status,
		Context:        contextInterface,
		StartedAt:      req.StartedAt,
		FinishedAt:     req.FinishedAt,
		Timestamp:      finishedAtTime,
		IdempotencyKey: req.IdempotencyKey,
		ContentHash:    contentHash,
		Metadata:       req.ThreadifyMetadata,
	}

	if err := s.planService.DecrementIngress(ctx, companyID, int64(len(reqBytes))); err != nil {
		return errResp("Insufficient credit: " + err.Error())
	}

	t = time.Now()
	if err := s.stepEventService.RecordStepEventDirect(ctx, stepEvent, ownerID, serviceName, req.SubSteps); err != nil {
		return errResp("failed to process step event")
	}
	metrics.OperationDuration.WithLabelValues(ActionRecordThreadEvent, "step_event_process").Observe(time.Since(t).Seconds())

	if len(req.Refs) > 0 {
		refsCtx, refsCancel := context.WithTimeout(ctx, 5*time.Second)
		err := s.repo.AddRefs(refsCtx, req.ThreadID, req.Refs)
		refsCancel()
		if err != nil {
			s.logger.Warn("failed to store refs", zap.String("thread", req.ThreadID), zap.Error(err))
		} else {
			s.logger.Info("stored refs", zap.Int("count", len(req.Refs)), zap.String("thread", req.ThreadID))
			go s.publishRefsToNATS(req.ThreadID, req.Refs)
		}
	}

	if req.Status == StepStatusSuccess || req.Status == StepStatusFailed || req.Status == StepStatusError {
		s.notificationService.PerformAsyncValidation(req.ThreadID, stepID, req.StepName, ownerID, req, thread, graph, stepNode)
	}

	perf.LogStructured("HandleRecordEvent COMPLETE", zap.Duration("duration", perf.Since(start)), zap.Bool("success", true))

	return &domain.RecordEventResponse{
		Action:   ActionRecordThreadEvent,
		Status:   StepStatusSuccess,
		Message:  "Step Event recorded successfully",
		ThreadID: req.ThreadID,
		StepID:   stepID,
	}
}

func (s *ThreadService) HandleInviteParty(req *domain.InvitePartyCmd, ownerID, companyID string, threadIDs []string) (*domain.InvitePartyResponse, error) {
	accessLevel := req.AccessLevel
	if accessLevel == "" {
		accessLevel = "external"
	}
	if err := s.invitationService.ValidateAccessLevel(accessLevel); err != nil {
		return nil, err
	}

	expiry, err := s.invitationService.ParseExpiry(req.ExpiresIn)
	if err != nil {
		return nil, fmt.Errorf("invalid expiry format: %v", err)
	}

	if len(threadIDs) == 0 {
		return nil, shderrors.ErrNoActiveThread
	}
	threadID := threadIDs[0]

	thread, err := s.getThread(threadID)
	if err != nil {
		return nil, shderrors.ErrThreadNotFound
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	hasInviteAccess, err := s.accessService.CheckThreadAccess(ctx, threadID, ownerID, "thread.invite", thread)
	if err != nil || !hasInviteAccess {
		return nil, shderrors.ErrAccessDenied
	}

	contractGraph, err := s.GetContractGraphForThread(thread)
	if err != nil {
		s.logger.Debug("no contract graph for thread", zap.String("thread", threadID), zap.Error(err))
	}

	if contractGraph != nil && len(contractGraph.Parties) > 0 && !slices.Contains(contractGraph.Parties, req.Role) {
		return nil, fmt.Errorf("role '%s' is not defined in contract parties: %v", req.Role, contractGraph.Parties)
	}

	threadToken, err := s.invitationService.CreateToken(threadID, ownerID, req.Role, accessLevel, expiry)
	if err != nil {
		s.logger.Error("failed to create invitation token", zap.Error(err))
		return nil, fmt.Errorf("failed to create invitation token")
	}

	return &domain.InvitePartyResponse{
		Action:      ActionInviteParty,
		Status:      StepStatusSuccess,
		ThreadToken: threadToken,
		Role:        req.Role,
		AccessLevel: accessLevel,
		ExpiresAt:   time.Now().Add(expiry).Unix(),
		Message:     "Invitation token created successfully",
	}, nil
}

func (s *ThreadService) HandleJoinThread(req *domain.JoinThreadCmd, ownerID, companyID string) (*domain.JoinThreadResponse, error) {
	var threadID, role, accessLevel, invitedBy string
	var thread *domain.Thread

	switch {
	case req.ThreadToken != "":
		claims, err := s.invitationService.ValidateToken(req.ThreadToken)
		if err != nil {
			return nil, ErrInvalidThreadToken
		}
		threadID = claims.ThreadID
		role = claims.Role
		accessLevel = claims.AccessLevel
		invitedBy = claims.InvitedBy

	case req.ThreadID != "":
		if req.Role == "" {
			req.Role = "participant"
		}
		var err error
		thread, err = s.getThread(req.ThreadID)
		if err != nil {
			return nil, shderrors.ErrThreadNotFound
		}
		if thread.CompanyID != companyID {
			return nil, fmt.Errorf("can only join threads from same company")
		}
		if !s.IsValidRole(req.Role) {
			return nil, fmt.Errorf("invalid role: %s", req.Role)
		}
		threadID = req.ThreadID
		role = req.Role
		invitedBy = companyID

	default:
		return nil, fmt.Errorf("either threadToken or (threadId + role) is required")
	}

	if thread == nil {
		var err error
		thread, err = s.getThread(threadID)
		if err != nil {
			return nil, shderrors.ErrThreadNotFound
		}
	}

	contractGraph, err := s.GetContractGraphForThread(thread)
	if err == nil && len(contractGraph.Parties) > 0 && !slices.Contains(contractGraph.Parties, role) {
		return nil, fmt.Errorf("role '%s' is not defined in contract parties: %v", role, contractGraph.Parties)
	}

	var explicitScope *string
	if accessLevel != "" {
		explicitScope = &accessLevel
	}
	if err := s.GrantOrUpdateThreadAccess(threadID, ownerID, role, invitedBy, false, explicitScope); err != nil {
		s.logger.Error("failed to grant access",
			zap.String("user_id", ownerID), zap.String("thread_id", threadID), zap.Error(err))
		return nil, fmt.Errorf("failed to grant access")
	}

	if accessLevel == "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if access, err := s.accessRepo.GetUserAccess(ctx, threadID, ownerID); err == nil && access != nil {
			accessLevel = access.RuntimeRole
		}
	}

	return &domain.JoinThreadResponse{
		Action:      ActionJoinThread,
		Status:      "success",
		ThreadID:    threadID,
		Role:        role,
		AccessLevel: accessLevel,
		Message:     "Successfully joined thread",
	}, nil
}

func (s *ThreadService) HandleClose(ownerID string) *domain.CloseConnectionResponse {
	if ownerID != "" {
		s.connectionMgr.Disconnect(ownerID)
	}
	return &domain.CloseConnectionResponse{
		Action:  ActionCloseConnection,
		Status:  "success",
		Message: "Connection closed successfully",
	}
}

func (s *ThreadService) HandleAddRefs(ctx context.Context, req *domain.AddRefsCmd, ownerID string) *domain.AddRefsResponse {
	errResp := func(msg string) *domain.AddRefsResponse {
		return &domain.AddRefsResponse{Action: ActionAddRefs, Status: "error", Message: msg}
	}

	if req.ThreadID == "" {
		return errResp("Thread ID is required")
	}
	if len(req.Refs) == 0 {
		return errResp("At least one ref is required")
	}

	authCtx, authCancel := context.WithTimeout(ctx, 5*time.Second)
	defer authCancel()
	thread, err := s.repo.Get(authCtx, req.ThreadID)
	if err != nil {
		return errResp("Thread not found")
	}

	hasWriteAccess, err := s.accessService.CheckThreadAccess(ctx, req.ThreadID, ownerID, "thread.write.*", thread)
	if err != nil {
		return errResp("Failed to verify permissions")
	}
	if !hasWriteAccess {
		return errResp("Access denied: write permission required")
	}

	refsCtx, refsCancel := context.WithTimeout(ctx, 5*time.Second)
	defer refsCancel()
	if err := s.repo.AddRefs(refsCtx, req.ThreadID, req.Refs); err != nil {
		return errResp(fmt.Sprintf("Failed to store refs: %v", err))
	}

	go s.publishRefsToNATS(req.ThreadID, req.Refs)

	return &domain.AddRefsResponse{
		Action:   ActionAddRefs,
		Status:   "success",
		Message:  fmt.Sprintf("Added %d refs to thread", len(req.Refs)),
		ThreadID: req.ThreadID,
	}
}

// EndThread marks a thread as cancelled or completed and records the activity.
// Contract-linked threads auto-complete on terminal state; manual completion is rejected for them.
func (s *ThreadService) EndThread(
	ctx context.Context,
	threadID, actorID, actorService, status, reason string,
	recordedAt time.Time,
) error {
	if status == "" {
		status = ThreadStatusCancelled
	}
	if status != ThreadStatusCancelled && status != ThreadStatusCompleted {
		return ErrInvalidThreadStatus
	}

	thread, err := s.repo.Get(ctx, threadID)
	if err != nil {
		return ErrFailedToGetThread
	}

	// Early exit: if Valkey already shows "completed" or "cancelled", return error.
	// The NotificationService updates Valkey synchronously on terminal step completion,
	// so this guard will catch duplicate EndThread calls.
	if thread.Status == domain.ThreadStatusCompleted || thread.Status == domain.ThreadStatusCancelled {
		s.logger.Debug("thread already terminal, rejecting EndThread",
			zap.String("thread_id", threadID),
			zap.String("current_status", string(thread.Status)))
		return ErrThreadAlreadyEnded
	}

	if status == ThreadStatusCompleted && thread.ContractID != nil && *thread.ContractID != "" {
		return ErrCannotManuallyCompleteThreadLinkedToContract
	}

	// 1. Update Valkey status SYNCHRONOUSLY using atomic Lua script.
	// This prevents race conditions with double-ending or concurrent terminal step completion.
	if err := s.repo.UpdateThreadStatus(ctx, threadID, status, recordedAt); err != nil {
		s.logger.Warn("failed to update thread status in Valkey",
			zap.String("thread_id", threadID), zap.Error(err))
		// Continue with archival even if Valkey update fails
	}

	// 2. Archive thread metadata.
	if s.natsArchivalPublisher != nil {
		pubCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		if err := s.natsArchivalPublisher.PublishThreadMetadata(pubCtx, map[string]interface{}{
			"threadId":    threadID,
			"ownerId":     actorID,
			"companyId":   thread.CompanyID,
			"status":      status,
			"startedAt":   recordedAt.Format(time.RFC3339Nano),
			"completedAt": recordedAt.Format(time.RFC3339Nano),
		}); err != nil {
			return fmt.Errorf("failed to publish thread completion: %w", err)
		}
	}

	// 3. Archive activity log.
	if s.natsArchivalPublisher != nil {
		pubCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		activityType := "thread_cancelled"
		if status == ThreadStatusCompleted {
			activityType = "thread_completed"
		}

		if err := s.natsArchivalPublisher.PublishActivityLog(pubCtx, map[string]interface{}{
			"threadId":     threadID,
			"type":         activityType,
			"actor":        actorID,
			"actorService": actorService,
			"timestamp":    recordedAt.Format(time.RFC3339Nano),
			"payload":      map[string]interface{}{"reason": reason, "status": status},
		}); err != nil {
			s.logger.Warn("failed to publish end activity", zap.String("threadId", threadID), zap.Error(err))
		}
	}

	// 4. Notify via worker pool.
	if s.notificationService != nil {
		notificationSource := domain.NotificationSourceThread
		notificationType := domain.NotificationType(fmt.Sprintf("thread.%s", status))

		// Special handling for cancellations: route as step.cancelled for WS subscriptions
		if status == string(domain.ThreadStatusCancelled) {
			notificationSource = domain.NotificationSourceStep
			notificationType = domain.NotificationTypeStepCancelled
		}

		s.notificationService.submitNotificationJob(domain.ValidationNotification{
			ThreadID:         threadID,
			StepName:         "global",
			OwnerID:          actorID,
			Timestamp:        recordedAt,
			StepStatus:       status,
			Status:           "none",
			Severity:         "info",
			Message:          "Thread " + status + ": " + reason,
			Source:           notificationSource,
			NotificationType: notificationType,
		})
	}

	return nil
}

// GetThread retrieves a thread from cache, then Valkey, then PostgreSQL.
// Egress is metered by default for caller-visible read paths.
func (s *ThreadService) GetThread(threadID string) (*domain.Thread, error) {
	return s.getThread(threadID)
}

// getThread is an internal variant that can skip egress metering for write-only paths.
func (s *ThreadService) getThread(threadID string) (*domain.Thread, error) {
	if thread, exists := s.cacheManager.GetThread(threadID); exists {
		return thread, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	thread, err := s.repo.Get(ctx, threadID, domain.ThreadReadOptions{WriteBack: true})
	if err != nil {
		return nil, fmt.Errorf("failed to load thread: %w", err)
	}

	s.cacheManager.SetThread(threadID, thread)
	return thread, nil
}

// GrantOrUpdateThreadAccess resolves a user's runtime_role and grants access.
func (s *ThreadService) GrantOrUpdateThreadAccess(threadID, userID, role, invitedBy string, isCreator bool, explicitScope *string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	runtimeRole, err := s.scopeResolver.ResolveScope(ctx, threadID, userID, role, isCreator, explicitScope)
	if err != nil {
		s.logger.Error("failed to resolve runtime_role",
			zap.String("user_id", userID), zap.String("thread_id", threadID), zap.Error(err))
		return fmt.Errorf("failed to resolve runtime_role: %w", err)
	}

	if runtimeRole == "" && (explicitScope == nil || *explicitScope != "") {
		s.logger.Error("empty runtime_role resolved",
			zap.String("user_id", userID), zap.String("thread_id", threadID),
			zap.Bool("is_creator", isCreator), zap.String("role", role))
		return fmt.Errorf("invalid runtime_role: cannot be empty")
	}

	if err := s.accessService.GrantOrUpdateAccess(ctx, threadID, userID, []string{role}, runtimeRole, invitedBy); err != nil {
		return fmt.Errorf("failed to grant access: %w", err)
	}

	accessCtx, accessCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer accessCancel()
	access, err := s.accessRepo.GetUserAccess(accessCtx, threadID, userID)
	if err != nil {
		s.logger.Warn("failed to get access for activity recording", zap.Error(err))
		access = nil
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		serviceName := ""
		if client, exists := s.connectionMgr.GetClient(userID); exists {
			serviceName = client.ServiceName
		}
		if err := s.activityRepo.RecordAccessGranted(ctx, threadID, userID, access, invitedBy, serviceName, runtimeRole); err != nil {
			s.logger.Warn("failed to record access granted activity", zap.Error(err))
		}
	}()

	return nil
}

// IsValidRole reports whether role is a known runtime-level role.
func (s *ThreadService) IsValidRole(role string) bool {
	if role == "" {
		return false
	}
	if s.rbacLoader == nil {
		return fallbackValidRoles[role]
	}
	return slices.Contains(s.rbacLoader.GetAllRuntimeLevelRoles(), role)
}

// GetContractGraphForThread fetches the contract graph for a given thread.
func (s *ThreadService) GetContractGraphForThread(thread *domain.Thread) (*domain.ContractGraph, error) {
	if thread.ContractName == "" {
		return nil, errors.New("thread has no contract")
	}
	version := 0
	if thread.ContractVersion != nil {
		version = *thread.ContractVersion
	}
	return s.contractValidator.GetContractGraph(thread.ContractName, version, thread.CompanyID)
}

func (s *ThreadService) GetContractValidator() domain.ContractGraphValidator {
	return s.contractValidator
}

// hasSuccessfulSteps reports whether the thread has any completed steps.
func (s *ThreadService) hasSuccessfulSteps(thread *domain.Thread) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count, err := s.repo.GetCompletedStepsCount(ctx, thread.ID, domain.ThreadReadOptions{WriteBack: true})
	if err != nil {
		return false
	}
	return count > 0
}

// publishThreadInitialArchivalAsync orchestrates the initial archival of thread metadata, access, and activity logs.
// It ensures that metadata is published first to satisfy foreign key constraints in the archiver.
func (s *ThreadService) publishThreadInitialArchivalAsync(threadID, ownerID, companyID string, thread *domain.Thread, role string, access *domain.UserAccess, runtimeRole string) {
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("panic in thread metadata goroutine",
				zap.Any("recover", r), zap.String("thread_id", threadID))
		}
	}()

	contractVersion := "0"
	if thread.ContractVersion != nil {
		contractVersion = fmt.Sprintf("%d", *thread.ContractVersion)
	}
	contractID := ""
	if thread.ContractID != nil {
		contractID = *thread.ContractID
	}

	if s.natsArchivalPublisher == nil {
		return
	}

	pubCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.natsArchivalPublisher.PublishThreadMetadata(pubCtx, map[string]interface{}{
		"threadId":        threadID,
		"label":           thread.Label,
		"ownerId":         ownerID,
		"companyId":       companyID,
		"contractId":      contractID,
		"contractName":    thread.ContractName,
		"contractVersion": contractVersion,
		"status":          string(thread.Status),
		"error":           "",
		"startedAt":       thread.StartedAt.Format(time.RFC3339),
	}); err != nil {
		s.logger.Error("failed to publish thread metadata to NATS", zap.Error(err))
	}

	if len(thread.Refs) > 0 {
		s.publishRefsToNATSWithContext(pubCtx, threadID, thread.Refs)
	}

	// 2. Publish Owner Access (subject: access.thread)
	if access != nil {
		rolesJSON, _ := json.Marshal(access.Roles)
		permissionsJSON, _ := json.Marshal(access.Permissions)

		if err := s.natsArchivalPublisher.PublishThreadAccess(pubCtx, map[string]interface{}{
			"threadId":     threadID,
			"userId":       ownerID,
			"roles":        string(rolesJSON),
			"runtime_role": runtimeRole,
			"permissions":  string(permissionsJSON),
			"grantedBy":    "self",
			"grantedAt":    access.GrantedAt,
			"status":       access.Status,
			"eventType":    "access_granted",
		}); err != nil {
			s.logger.Error("failed to publish owner access to NATS", zap.Error(err))
		}
	}

	// 3. Publish Activity Log (subject: activity.log)
	serviceName := ""
	if client, exists := s.connectionMgr.GetClient(ownerID); exists {
		serviceName = client.ServiceName
	}

	if err := s.natsArchivalPublisher.PublishActivityLog(pubCtx, map[string]interface{}{
		"type":            "thread_created",
		"threadId":        threadID,
		"ownerId":         ownerID,
		"actor":           ownerID,
		"actorService":    serviceName,
		"contractId":      contractID,
		"contractName":    thread.ContractName,
		"contractVersion": contractVersion,
		"role":            role,
		"timestamp":       thread.StartedAt.Format(time.RFC3339),
	}); err != nil {
		s.logger.Error("failed to publish activity log to NATS", zap.Error(err))
	}
}

// publishRefsToNATS spawns a goroutine to publish refs to NATS for archival.
func (s *ThreadService) publishRefsToNATS(threadID string, refs map[string]string) {
	if s.natsArchivalPublisher == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.publishRefsToNATSWithContext(ctx, threadID, refs)
	}()
}

// publishRefsToNATSWithContext publishes each ref as a NATS event using the provided context.
func (s *ThreadService) publishRefsToNATSWithContext(ctx context.Context, threadID string, refs map[string]string) {
	for key, value := range refs {
		if err := s.natsArchivalPublisher.PublishThreadMetadata(ctx, map[string]interface{}{
			"threadId": threadID,
			"refKey":   key,
			"refValue": value,
			"action":   "ref_added",
		}); err != nil {
			s.logger.Error("failed to publish ref to NATS",
				zap.String("key", key), zap.String("thread_id", threadID), zap.Error(err))
		}
	}
}

// --- package-level helpers ---

// parseContractIdentifier parses "name" or "name:version" into its parts.
// Returns version 0 if no valid version suffix is present.
func parseContractIdentifier(identifier string) (name string, version int) {
	if idx := strings.LastIndex(identifier, ":"); idx != -1 {
		if v, err := strconv.Atoi(identifier[idx+1:]); err == nil && v > 0 {
			return identifier[:idx], v
		}
	}
	return identifier, 0
}

// ParseContractIdentifier is an exported wrapper around parseContractIdentifier (primarily for tests).
func ParseContractIdentifier(identifier string) (name string, version int) {
	return parseContractIdentifier(identifier)
}

// validateRecordEventRequest checks that all required fields are present.
func validateRecordEventRequest(req *domain.RecordEventCmd) error {
	switch {
	case req.ThreadID == "":
		return errors.New("Thread ID is required")
	case req.StepName == "":
		return errors.New("StepName is required")
	case req.Status == "":
		return errors.New("Status is required")
	case req.StartedAt == "":
		return errors.New("StartedAt is required")
	case req.FinishedAt == "":
		return errors.New("FinishedAt is required")
	case req.Context == nil:
		return errors.New("Context is required")
	}
	return nil
}

// ValidateRecordEventRequest is an exported wrapper around validateRecordEventRequest (primarily for tests).
func ValidateRecordEventRequest(req *domain.RecordEventCmd) error {
	return validateRecordEventRequest(req)
}
