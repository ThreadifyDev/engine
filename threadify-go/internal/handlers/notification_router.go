package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/rand"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/gorilla/websocket"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/threadify/engine/internal/config"
	"github.com/threadify/engine/internal/metrics"
	"github.com/threadify/engine/internal/models"
	natsrepo "github.com/threadify/engine/internal/repository/nats"
)

type ClientSubscription struct {
	StepName     string
	ContractName string
	EventTypes   []string
}

type Session struct {
	ID            string
	OwnerID       string
	MaxInFlight   int
	Conn          *websocket.Conn
	Subscriptions map[string]*ClientSubscription
	mu            sync.RWMutex
	sendMu        *sync.Mutex
}

// WebSocketClient is an alias for Session for backward compatibility.
type WebSocketClient = Session

type NotificationRouter struct {
	nc                 *nats.Conn
	js                 jetstream.JetStream
	sessions           map[string]*Session
	consumers          map[string]jetstream.Consumer
	sessionsByOwner    map[string][]string
	subscriptionIndex  map[string]map[string][]string
	ownerContexts      map[string]context.Context
	ownerCancels       map[string]context.CancelFunc
	ownerConsumerCount map[string]int
	mu                 sync.RWMutex
	ctx                context.Context
	cancel             context.CancelFunc
	natsConfig         *config.NATSConfig
	logger             *zap.Logger
}

func NewNotificationRouter(nc *nats.Conn, natsConfig *config.NATSConfig, logger *zap.Logger) (*NotificationRouter, error) {
	js, err := jetstream.New(nc)
	if err != nil {
		return nil, fmt.Errorf("create jetstream context: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	if _, err = js.Stream(ctx, natsConfig.StreamName); err != nil {
		cancel()
		return nil, fmt.Errorf("notifications stream not found: %w", err)
	}

	return &NotificationRouter{
		nc:                 nc,
		js:                 js,
		sessions:           make(map[string]*Session),
		consumers:          make(map[string]jetstream.Consumer),
		sessionsByOwner:    make(map[string][]string),
		subscriptionIndex:  make(map[string]map[string][]string),
		ownerContexts:      make(map[string]context.Context),
		ownerCancels:       make(map[string]context.CancelFunc),
		ownerConsumerCount: make(map[string]int),
		ctx:                ctx,
		cancel:             cancel,
		natsConfig:         natsConfig,
		logger:             logger,
	}, nil
}

func (r *NotificationRouter) HandleConnect(sessionID, ownerID string, maxInFlight int, conn *websocket.Conn, connMutex *sync.Mutex) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.ownerConsumerCount[ownerID] >= 100 {
		metrics.RateLimitExceeded.WithLabelValues(ownerID).Inc()
		return fmt.Errorf("rate limit exceeded: owner %s has 100 active consumers", ownerID)
	}

	r.sessions[sessionID] = &Session{
		ID:            sessionID,
		OwnerID:       ownerID,
		MaxInFlight:   maxInFlight,
		Conn:          conn,
		Subscriptions: make(map[string]*ClientSubscription),
		sendMu:        connMutex,
	}
	r.sessionsByOwner[ownerID] = append(r.sessionsByOwner[ownerID], sessionID)

	if _, exists := r.consumers[ownerID]; !exists {
		consumerName := fmt.Sprintf("owner-%s", ownerID)

		maxAckPending := r.natsConfig.ConsumerMaxAckPending
		if maxAckPending == 0 {
			maxAckPending = maxInFlight
		}

		consumer, err := r.js.CreateOrUpdateConsumer(r.ctx, r.natsConfig.StreamName, jetstream.ConsumerConfig{
			Name:              consumerName,
			Durable:           consumerName,
			FilterSubjects:    []string{},
			AckPolicy:         jetstream.AckExplicitPolicy,
			MaxAckPending:     maxAckPending,
			AckWait:           time.Duration(r.natsConfig.ConsumerAckWaitSeconds) * time.Second,
			MaxDeliver:        r.natsConfig.ConsumerMaxDeliver,
			DeliverPolicy:     jetstream.DeliverAllPolicy,
			InactiveThreshold: 0,
			Description:       fmt.Sprintf("Owner consumer for %s", ownerID),
		})
		if err != nil {
			return fmt.Errorf("create consumer: %w", err)
		}

		r.consumers[ownerID] = consumer
		r.ownerConsumerCount[ownerID] = 1

		ctx, cancel := context.WithCancel(r.ctx)
		r.ownerContexts[ownerID] = ctx
		r.ownerCancels[ownerID] = cancel

		go r.routeNotificationsForOwner(ownerID)

		metrics.ConsumerCreated.WithLabelValues(ownerID).Inc()
		metrics.ActiveConsumers.WithLabelValues(ownerID).Set(1)
	} else {
		r.updateConsumerMaxAckPending(ownerID) //nolint:errcheck
	}

	return nil
}

func (r *NotificationRouter) HandleDisconnect(sessionID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	session, exists := r.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	ownerID := session.OwnerID
	consumerName := fmt.Sprintf("owner-%s", ownerID)

	delete(r.sessions, sessionID)

	sessions := r.sessionsByOwner[ownerID]
	for i, sid := range sessions {
		if sid == sessionID {
			r.sessionsByOwner[ownerID] = append(sessions[:i], sessions[i+1:]...)
			break
		}
	}

	if ownerSubs, exists := r.subscriptionIndex[ownerID]; exists {
		for key, sessionIDs := range ownerSubs {
			for i, sid := range sessionIDs {
				if sid == sessionID {
					r.subscriptionIndex[ownerID][key] = append(sessionIDs[:i], sessionIDs[i+1:]...)
					break
				}
			}
			if len(r.subscriptionIndex[ownerID][key]) == 0 {
				delete(r.subscriptionIndex[ownerID], key)
			}
		}
		if len(r.subscriptionIndex[ownerID]) == 0 {
			delete(r.subscriptionIndex, ownerID)
		}
	}

	if len(r.sessionsByOwner[ownerID]) == 0 {
		delete(r.sessionsByOwner, ownerID)

		if cancel := r.ownerCancels[ownerID]; cancel != nil {
			cancel()
		}
		delete(r.ownerContexts, ownerID)
		delete(r.ownerCancels, ownerID)

		if err := r.js.DeleteConsumer(r.ctx, r.natsConfig.StreamName, consumerName); err != nil {
			r.logger.Warn("failed to delete consumer", zap.String("consumer", consumerName), zap.Error(err))
		}

		delete(r.consumers, ownerID)
		r.ownerConsumerCount[ownerID]--

		metrics.ConsumerDeleted.WithLabelValues(ownerID).Inc()
		metrics.ActiveConsumers.WithLabelValues(ownerID).Set(0)
	} else {
		r.updateConsumerMaxAckPending(ownerID) //nolint:errcheck
	}

	return nil
}

func (r *NotificationRouter) HandleSubscribe(sessionID, stepName, contract string) error {
	r.mu.RLock()
	session, exists := r.sessions[sessionID]
	r.mu.RUnlock()

	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	session.mu.Lock()
	if session.Subscriptions == nil {
		session.Subscriptions = make(map[string]*ClientSubscription)
	}
	session.Subscriptions[stepName] = &ClientSubscription{
		StepName:     stepName,
		ContractName: contract,
		EventTypes:   []string{},
	}
	session.mu.Unlock()

	subscriptionKey := buildSubscriptionKey(stepName, contract)

	r.mu.Lock()
	if r.subscriptionIndex[session.OwnerID] == nil {
		r.subscriptionIndex[session.OwnerID] = make(map[string][]string)
	}
	sessions := r.subscriptionIndex[session.OwnerID][subscriptionKey]
	if !slices.Contains(sessions, sessionID) {
		r.subscriptionIndex[session.OwnerID][subscriptionKey] = append(sessions, sessionID)
	}
	err := r.updateConsumerMaxAckPending(session.OwnerID)
	r.mu.Unlock()

	if err != nil {
		return fmt.Errorf("update consumer: %w", err)
	}
	return nil
}

func (r *NotificationRouter) routeNotificationsForOwner(ownerID string) {
	r.mu.RLock()
	consumer := r.consumers[ownerID]
	ownerCtx := r.ownerContexts[ownerID]
	r.mu.RUnlock()

	if consumer == nil || ownerCtx == nil {
		return
	}

	consumeCtx, err := consumer.Consume(func(msg jetstream.Msg) {
		var notification models.ValidationNotification
		if err := json.Unmarshal(msg.Data(), &notification); err != nil {
			r.logger.Error("failed to parse notification", zap.Error(err))
			msg.Ack()
			return
		}

		r.mu.RLock()
		matchingSessions := r.getMatchingSessions(ownerID, notification.StepName, notification.ContractName)
		r.mu.RUnlock()

		if len(matchingSessions) == 0 {
			msg.Ack()
			return
		}

		targetSessionID := matchingSessions[rand.Intn(len(matchingSessions))]

		r.mu.RLock()
		targetSession := r.sessions[targetSessionID]
		r.mu.RUnlock()

		if targetSession == nil {
			r.logger.Warn("target session not found", zap.String("session_id", targetSessionID))
			msg.Nak()
			return
		}

		metadata, err := msg.Metadata()
		if err != nil {
			r.logger.Error("failed to get message metadata", zap.Error(err))
			msg.Nak()
			return
		}

		if err := targetSession.sendNotificationWithAckToken(&notification, createAckToken(metadata.Sequence.Stream, msg.Reply())); err != nil {
			r.logger.Warn("failed to send notification", zap.String("session_id", targetSessionID), zap.Error(err))
			msg.Nak()
			return
		}

		metrics.NotificationsSent.WithLabelValues(
			notification.OwnerID,
			notification.StepName,
			notification.ContractName,
			notification.Status,
		).Inc()
	})
	if err != nil {
		r.logger.Error("failed to start consumer", zap.String("owner_id", ownerID), zap.Error(err))
		return
	}

	<-ownerCtx.Done()
	if consumeCtx != nil {
		consumeCtx.Stop()
	}
}

func (r *NotificationRouter) HandleAck(ackToken string) error {
	sequence, replySubject, err := decodeAckToken(ackToken)
	if err != nil {
		return fmt.Errorf("invalid ACK token: %w", err)
	}

	if err := r.nc.Publish(replySubject, []byte("+ACK")); err != nil {
		r.logger.Error("failed to ACK NATS message", zap.Uint64("seq", sequence), zap.Error(err))
		return fmt.Errorf("ACK NATS message: %w", err)
	}

	metrics.NotificationsAcked.WithLabelValues("unknown").Inc()
	return nil
}

func (r *NotificationRouter) Stop() {
	r.cancel()
}

// SendMessage is the only method that should be used for WebSocket writes.
func (s *Session) SendMessage(message interface{}) error {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	return s.Conn.WriteJSON(message)
}

func (c *WebSocketClient) sendNotificationWithAckToken(notification *models.ValidationNotification, ackToken string) error {
	return c.SendMessage(map[string]interface{}{
		"action":       "notification",
		"ackToken":     ackToken,
		"notification": notification,
	})
}

func buildSubscriptionKey(stepName, contract string) string {
	if contract != "" {
		return fmt.Sprintf("%s@%s", stepName, contract)
	}
	return fmt.Sprintf("%s@*", stepName)
}

func (r *NotificationRouter) getMatchingSessions(ownerID, stepName, contract string) []string {
	if contract == "" {
		contract = "global"
	}
	specific := r.subscriptionIndex[ownerID][buildSubscriptionKey(stepName, contract)]
	wildcard := r.subscriptionIndex[ownerID][buildSubscriptionKey(stepName, "")]
	return append(append([]string{}, specific...), wildcard...)
}

func (r *NotificationRouter) calculateMaxAckPending(ownerID string) int {
	total := 0
	for _, sessionID := range r.sessionsByOwner[ownerID] {
		if session := r.sessions[sessionID]; session != nil {
			total += session.MaxInFlight
		}
	}
	if total == 0 {
		return 10
	}
	return total
}

func (r *NotificationRouter) updateConsumerMaxAckPending(ownerID string) error {
	maxAckPending := r.natsConfig.ConsumerMaxAckPending
	if maxAckPending == 0 {
		maxAckPending = r.calculateMaxAckPending(ownerID)
	}

	_, err := r.js.UpdateConsumer(r.ctx, r.natsConfig.StreamName, jetstream.ConsumerConfig{
		Name:              fmt.Sprintf("owner-%s", ownerID),
		Durable:           fmt.Sprintf("owner-%s", ownerID),
		FilterSubjects:    r.buildUnionFilterSubjects(ownerID),
		AckPolicy:         jetstream.AckExplicitPolicy,
		MaxAckPending:     maxAckPending,
		AckWait:           time.Duration(r.natsConfig.ConsumerAckWaitSeconds) * time.Second,
		MaxDeliver:        r.natsConfig.ConsumerMaxDeliver,
		DeliverPolicy:     jetstream.DeliverAllPolicy,
		InactiveThreshold: 0,
	})
	return err
}

func (r *NotificationRouter) buildUnionFilterSubjects(ownerID string) []string {
	filterMap := make(map[string]bool)
	for _, sessionID := range r.sessionsByOwner[ownerID] {
		session := r.sessions[sessionID]
		if session == nil {
			continue
		}
		session.mu.RLock()
		for _, sub := range session.Subscriptions {
			var subject string
			if sub.ContractName != "" {
				subject = fmt.Sprintf("%s.%s.%s.%s", natsrepo.PrefixNotificationsUser, ownerID, sub.ContractName, sub.StepName)
			} else {
				subject = fmt.Sprintf("%s.%s.*.%s", natsrepo.PrefixNotificationsUser, ownerID, sub.StepName)
			}
			filterMap[subject] = true
		}
		session.mu.RUnlock()
	}

	result := make([]string, 0, len(filterMap))
	for f := range filterMap {
		result = append(result, f)
	}
	return result
}

func createAckToken(sequence uint64, replySubject string) string {
	return base64.URLEncoding.EncodeToString([]byte(strconv.FormatUint(sequence, 10) + ":" + replySubject))
}

func decodeAckToken(token string) (uint64, string, error) {
	decoded, err := base64.URLEncoding.DecodeString(token)
	if err != nil {
		return 0, "", fmt.Errorf("invalid token encoding: %w", err)
	}
	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return 0, "", fmt.Errorf("invalid token format")
	}
	sequence, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil {
		return 0, "", fmt.Errorf("invalid sequence: %w", err)
	}
	return sequence, parts[1], nil
}
