package threadify

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

type NotificationHandler func(*Notification)

type Connection struct {
	transport   Transport
	apiKey      string
	serviceName string
	graphqlURL  string
	debug       bool
	maxInFlight int

	mu          sync.Mutex
	isConnected bool

	threads sync.Map

	notificationHandlers sync.Map

	activeSubscriptions sync.Map

	processedNotifications sync.Map
	processedCount         int64

	dataRetriever     *DataRetriever
	dataRetrieverOnce sync.Once

	recvCh   chan map[string]any
	stopOnce sync.Once
	stopCh   chan struct{}
}

func newConnection(transport Transport, apiKey, serviceName string, opts ConnectOptions) *Connection {
	c := &Connection{
		transport:   transport,
		apiKey:      apiKey,
		serviceName: serviceName,
		graphqlURL:  opts.GraphQLURL,
		debug:       opts.Debug,
		maxInFlight: opts.MaxInFlight,
		isConnected: true,
		recvCh:      make(chan map[string]any, 256),
		stopCh:      make(chan struct{}),
	}
	go c.readLoop()
	return c
}

func (c *Connection) readLoop() {
	defer func() {
		c.mu.Lock()
		c.isConnected = false
		c.mu.Unlock()
	}()

	for {
		select {
		case <-c.stopCh:
			return
		default:
		}

		msg, err := c.transport.Recv()
		if err != nil {
			debugLog(c.debug, "readLoop recv error: %v", err)
			return
		}

		action := asString(msg[FieldAction])

		switch action {
		case ActionNotification:
			c.handleNotification(asMap(msg[ActionNotification]), asString(msg[FieldAckToken]))
		case ActionNotificationBatch:
			if notifs := asSlice(msg["notifications"]); notifs != nil {
				for _, n := range notifs {
					if nd, ok := n.(map[string]any); ok {
						c.handleNotification(nd, "")
					}
				}
			}
		default:
			select {
			case c.recvCh <- msg:
			default:
				debugLog(c.debug, "recvCh full, dropping message: %s", action)
			}
		}
	}
}

func (c *Connection) waitResponse(ctx context.Context, match func(map[string]any) bool) (map[string]any, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case msg, ok := <-c.recvCh:
			if !ok {
				return nil, fmt.Errorf("connection closed")
			}
			if match(msg) {
				return msg, nil
			}
			// Not our message, put it back for other waiters.
			select {
			case c.recvCh <- msg:
			default:
			}
		}
	}
}

func (c *Connection) send(msg map[string]any) error {
	c.mu.Lock()
	connected := c.isConnected
	c.mu.Unlock()

	if !connected {
		return fmt.Errorf("WebSocket is not connected")
	}
	return c.transport.Send(msg)
}

func (c *Connection) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.isConnected
}

func (c *Connection) Start(ctx context.Context, opts ...StartOption) (*ThreadInstance, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("not connected. Call Connect() first")
	}

	cfg := startConfig{}
	for _, o := range opts {
		o(&cfg)
	}

	msg := map[string]any{
		FieldAction: ActionStartThread,
		FieldRefs: map[string]any{
			FieldService: firstNonEmpty(cfg.serviceName, c.serviceName),
		},
	}

	if cfg.contractName != "" {
		msg[FieldContractName] = cfg.contractName
		effectiveService := firstNonEmpty(cfg.serviceName, c.serviceName)
		if cfg.role != "" {
			msg[FieldRole] = cfg.role
		} else if effectiveService != "" {
			msg[FieldRole] = strings.TrimSuffix(effectiveService, "-service")
		} else {
			msg[FieldRole] = "participant"
		}
	}

	if err := c.send(msg); err != nil {
		return nil, err
	}

	resp, err := c.waitResponse(ctx, func(m map[string]any) bool {
		return asString(m[FieldAction]) == ActionStartThread
	})
	if err != nil {
		return nil, fmt.Errorf("start thread: %w", err)
	}

	if asString(resp[FieldStatus]) != StatusSuccess {
		return nil, fmt.Errorf("%s", firstNonEmpty(asString(resp[FieldMessage]), "failed to start thread"))
	}

	threadID := asString(resp[FieldThreadID])
	thread := newThreadInstance(c, threadID, cfg.contractName, "", nil)
	c.threads.Store(threadID, thread)
	debugLog(c.debug, "Thread started: %s", threadID)
	return thread, nil
}

type startConfig struct {
	contractName string
	serviceName  string
	role         string
}

type StartOption func(*startConfig)

func WithContract(name string) StartOption {
	return func(c *startConfig) { c.contractName = name }
}

func WithService(name string) StartOption {
	return func(c *startConfig) { c.serviceName = name }
}
func WithRole(role string) StartOption {
	return func(c *startConfig) { c.role = role }
}

type JoinOption func(*joinConfig)

type joinConfig struct {
	token    string
	threadID string
	role     string
}

func WithJoinToken(token string) JoinOption {
	return func(c *joinConfig) {
		c.token = token
	}
}

func WithJoinThreadID(threadID string) JoinOption {
	return func(c *joinConfig) {
		c.threadID = threadID
	}
}

func WithJoinRole(role string) JoinOption {
	return func(c *joinConfig) {
		c.role = role
	}
}

func (c *Connection) Join(ctx context.Context, opts ...JoinOption) (*ThreadInstance, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("not connected. Call Connect() first")
	}

	cfg := &joinConfig{}
	for _, o := range opts {
		o(cfg)
	}

	msg := map[string]any{
		FieldAction: ActionJoinThread,
	}

	if cfg.token != "" {
		msg[FieldThreadToken] = cfg.token
		debugLog(c.debug, "Joining thread with token: %s...", cfg.token[:min(20, len(cfg.token))])
	} else if cfg.threadID != "" {
		if cfg.role == "" {
			return nil, fmt.Errorf("role is required when joining by thread ID")
		}
		msg[FieldThreadID] = cfg.threadID
		msg[FieldRole] = cfg.role
		debugLog(c.debug, "Joining thread directly: %s as %s", cfg.threadID, cfg.role)
	} else {
		return nil, fmt.Errorf("either WithJoinToken or WithJoinThreadID+WithJoinRole must be provided")
	}

	if err := c.send(msg); err != nil {
		return nil, err
	}

	resp, err := c.waitResponse(ctx, func(m map[string]any) bool {
		return asString(m[FieldAction]) == ActionJoinThread
	})
	if err != nil {
		return nil, fmt.Errorf("join thread: %w", err)
	}

	if asString(resp[FieldStatus]) != StatusSuccess {
		return nil, fmt.Errorf("%s", firstNonEmpty(asString(resp[FieldMessage]), "failed to join thread"))
	}

	threadID := asString(resp[FieldThreadID])
	threadRole := asString(resp[FieldRole])
	thread := newThreadInstance(c, threadID, asString(resp[FieldContractID]), threadRole, nil)
	c.threads.Store(threadID, thread)
	debugLog(c.debug, "Joined thread: %s, Role: %s", threadID, threadRole)
	return thread, nil
}

// Close closes the WebSocket connection.
func (c *Connection) Close() error {
	c.stopOnce.Do(func() {
		close(c.stopCh)
	})

	c.mu.Lock()
	c.isConnected = false
	c.mu.Unlock()

	return c.transport.Close()
}

// Subscribe registers a notification handler for a specific event and step.
func (c *Connection) Subscribe(ctx context.Context, event, stepName string, handler NotificationHandler) error {
	if handler == nil {
		return fmt.Errorf("handler cannot be nil")
	}

	source, eventType := parseEvent(event)
	eventTypes := buildEventTypes(source, eventType)
	if err := c.sendSubscription(stepName, eventTypes); err != nil {
		return err
	}

	handlerKey := event + ":" + stepName
	val, _ := c.notificationHandlers.LoadOrStore(handlerKey, &handlerList{})
	hl := val.(*handlerList)
	hl.mu.Lock()
	hl.handlers = append(hl.handlers, handler)
	hl.mu.Unlock()

	return nil
}

func (c *Connection) Unsubscribe(ctx context.Context, event, stepName string) error {
	handlerKey := event + ":" + stepName
	c.notificationHandlers.Delete(handlerKey)

	hasHandlers := false
	c.notificationHandlers.Range(func(key, _ any) bool {
		if strings.HasSuffix(key.(string), ":"+stepName) {
			hasHandlers = true
			return false
		}
		return true
	})

	if !hasHandlers {
		if err := c.sendUnsubscription(stepName); err != nil {
			return err
		}
	}

	return nil
}

type handlerList struct {
	mu       sync.RWMutex
	handlers []NotificationHandler
}

func (c *Connection) sendSubscription(stepName string, eventTypes []string) error {
	if !c.IsConnected() {
		return fmt.Errorf("not connected")
	}

	existing := []string{}
	if val, ok := c.activeSubscriptions.Load(stepName); ok {
		existing = val.([]string)
	}

	merged := mergeUnique(existing, eventTypes)
	if sameElements(existing, merged) {
		return nil
	}

	err := c.send(map[string]any{
		FieldAction:     ActionSubscribe,
		FieldStepName:   stepName,
		FieldEventTypes: merged,
	})
	if err != nil {
		return err
	}

	c.activeSubscriptions.Store(stepName, merged)
	return nil
}

func (c *Connection) sendUnsubscription(stepName string) error {
	if !c.IsConnected() {
		return fmt.Errorf("not connected")
	}

	err := c.send(map[string]any{
		FieldAction:   ActionUnsubscribe,
		FieldStepName: stepName,
	})
	if err != nil {
		return err
	}

	c.activeSubscriptions.Delete(stepName)
	return nil
}

func (c *Connection) handleNotification(data map[string]any, ackToken string) {
	if data == nil {
		return
	}

	notifID := asString(data[FieldNotificationID])

	// Deduplicate.
	if _, loaded := c.processedNotifications.LoadOrStore(notifID, struct{}{}); loaded {
		debugLog(c.debug, "Duplicate notification ignored: %s", notifID)
		c.sendAck(notifID, asString(data[FieldThreadID]), ackToken)
		return
	}

	notif := NewNotification(data, c, ackToken)

	// Trigger matching handlers.
	eventPattern := c.getEventPattern(notif)
	c.triggerHandlers(eventPattern, notif)

	// Route to thread-specific handler.
	if val, ok := c.threads.Load(notif.ThreadID); ok {
		thread := val.(*ThreadInstance)
		thread.handleNotification(notif)
	}
}

func (c *Connection) getEventPattern(notif *Notification) string {
	source := notif.Source
	if source == "" {
		source = "execution"
	}

	eventType := "success"
	if notif.NotificationType != "" {
		parts := strings.SplitN(notif.NotificationType, ".", 2)
		if len(parts) == 2 {
			eventType = parts[1]
		}
	}

	sourceMap := map[string]string{
		"execution":  "step",
		"validation": "rule",
		"thread":     "thread",
	}

	sdkSource, ok := sourceMap[source]
	if !ok {
		sdkSource = source
	}

	return sdkSource + "." + eventType
}

func (c *Connection) triggerHandlers(eventPattern string, notif *Notification) {
	stepName := notif.StepName
	contractName := notif.ContractName
	source := strings.SplitN(eventPattern, ".", 2)[0]

	keysToCheck := []string{}

	// 1. Exact match: "event:contract@stepName"
	if contractName != "" {
		keysToCheck = append(keysToCheck, fmt.Sprintf("%s:%s@%s", eventPattern, contractName, stepName))
	}

	// 2. Wildcard contract: "event:stepName"
	keysToCheck = append(keysToCheck, fmt.Sprintf("%s:%s", eventPattern, stepName))

	// 3. Wildcard type: "source.*:stepName"
	keysToCheck = append(keysToCheck, fmt.Sprintf("%s.*:%s", source, stepName))

	// 4. Full wildcard: "*:stepName"
	keysToCheck = append(keysToCheck, fmt.Sprintf("*:%s", stepName))

	for _, key := range keysToCheck {
		val, ok := c.notificationHandlers.Load(key)
		if !ok {
			continue
		}
		hl := val.(*handlerList)
		hl.mu.RLock()
		handlers := make([]NotificationHandler, len(hl.handlers))
		copy(handlers, hl.handlers)
		hl.mu.RUnlock()

		for _, h := range handlers {
			func() {
				defer func() {
					if r := recover(); r != nil {
						debugLog(c.debug, "Notification handler panic: %v", r)
					}
				}()
				h(notif)
			}()
		}
	}
}

// sendAck acknowledges a notification on the server.
func (c *Connection) sendAck(notificationID, threadID, ackToken string) {
	if ackToken == "" {
		debugLog(c.debug, "Cannot ACK: ackToken is required")
		return
	}

	_ = c.send(map[string]any{
		FieldAction:          ActionAckNotification,
		FieldNotification_ID: notificationID,
		FieldThread_ID:       threadID,
		FieldAckToken:        ackToken,
		FieldProcessed:       true,
	})

	debugLog(c.debug, "ACK sent: %s", notificationID)
}

// --- Data retrieval ---

// getDataRetriever returns the lazily-initialised DataRetriever.
func (c *Connection) getDataRetriever() (*DataRetriever, error) {
	var initErr error
	c.dataRetrieverOnce.Do(func() {
		if c.graphqlURL == "" {
			initErr = fmt.Errorf("GraphQL URL not configured")
			return
		}
		c.dataRetriever = NewDataRetriever(c.graphqlURL, c.apiKey)
	})
	if initErr != nil {
		return nil, initErr
	}
	return c.dataRetriever, nil
}

func (c *Connection) GetThread(ctx context.Context, threadID string) (*ArchivedThread, error) {
	dr, err := c.getDataRetriever()
	if err != nil {
		return nil, err
	}
	return dr.GetThread(ctx, threadID)
}

func (c *Connection) GetThreadsByRef(ctx context.Context, query RefQuery) ([]*ArchivedThread, error) {
	dr, err := c.getDataRetriever()
	if err != nil {
		return nil, err
	}
	return dr.GetThreadsByRef(ctx, query)
}

func (c *Connection) GetThreadChain(ctx context.Context, rootID string, maxDepth int) ([]*ArchivedThread, error) {
	dr, err := c.getDataRetriever()
	if err != nil {
		return nil, err
	}
	return dr.GetThreadChain(ctx, rootID, maxDepth)
}

// --- Helpers ---

// parseEvent splits an event string into source and type.
func parseEvent(event string) (string, string) {
	normalized := strings.Replace(event, "step", "execution", 1)
	normalized = strings.Replace(normalized, "rule", "validation", 1)

	parts := strings.SplitN(normalized, ".", 2)
	source := "*"
	eventType := "*"
	if len(parts) >= 1 && parts[0] != "" {
		source = parts[0]
	}
	if len(parts) >= 2 && parts[1] != "" {
		eventType = parts[1]
	}
	return source, eventType
}

// buildEventTypes returns the subscription event types for a source+type pair.
func buildEventTypes(source, eventType string) []string {
	if source == "*" && eventType == "*" {
		return []string{"execution.success", "execution.failed", "validation.passed", "validation.violated"}
	}
	if source == "execution" && eventType == "*" {
		return []string{"execution.success", "execution.failed"}
	}
	if source == "validation" && eventType == "*" {
		return []string{"validation.passed", "validation.violated"}
	}
	return []string{source + "." + eventType}
}

// mergeUnique merges two slices, removing duplicates.
func mergeUnique(a, b []string) []string {
	seen := make(map[string]struct{}, len(a)+len(b))
	for _, s := range a {
		seen[s] = struct{}{}
	}
	for _, s := range b {
		seen[s] = struct{}{}
	}
	result := make([]string, 0, len(seen))
	for s := range seen {
		result = append(result, s)
	}
	return result
}

// sameElements checks if two unsorted slices contain the same elements.
func sameElements(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := make(map[string]int, len(a))
	for _, s := range a {
		m[s]++
	}
	for _, s := range b {
		m[s]--
		if m[s] < 0 {
			return false
		}
	}
	return true
}
