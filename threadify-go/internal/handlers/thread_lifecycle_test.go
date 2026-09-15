package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/config"
	"github.com/threadify/engine/internal/domain"
	"go.uber.org/zap"
)

type blockingConnectService struct {
	domain.ThreadService
	entered chan struct{}
	release chan struct{}
}

func (s *blockingConnectService) HandleConnect(context.Context, *domain.ConnectCmd) *domain.ConnectResponse {
	close(s.entered)
	<-s.release
	return &domain.ConnectResponse{Action: ActionConnect, Status: StatusError}
}

func lifecycleWebSocketServer(t *testing.T, svc domain.ThreadService) (*WebSocketHandler, *httptest.Server) {
	t.Helper()
	handler := NewWebSocketHandler(svc, nil, nil, nil, nil, nil, nil, nil,
		&config.RateLimitConfig{}, &config.WebSocketConfig{ReadDeadlineSeconds: 60}, zap.NewNop())
	router := gin.New()
	router.GET("/threads", handler.HandleWebSocket)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	return handler, server
}

func TestWebSocketShutdownClosesUnauthenticatedConnections(t *testing.T) {
	handler, server := lifecycleWebSocketServer(t, nil)
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/threads", nil)
	require.NoError(t, err)
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, handler.Shutdown(ctx))
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	_, _, err = conn.ReadMessage()
	require.Error(t, err)
	require.NoError(t, handler.Shutdown(ctx), "shutdown is idempotent")

	response, err := http.Get(server.URL + "/threads")
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusServiceUnavailable, response.StatusCode)
}

func TestWebSocketShutdownWaitsForInFlightHandler(t *testing.T) {
	svc := &blockingConnectService{entered: make(chan struct{}), release: make(chan struct{})}
	handler, server := lifecycleWebSocketServer(t, svc)
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/threads", nil)
	require.NoError(t, err)
	defer conn.Close()
	require.NoError(t, conn.WriteJSON(map[string]string{"action": ActionConnect, "apiKey": "test"}))
	select {
	case <-svc.entered:
	case <-time.After(time.Second):
		t.Fatal("connect handler never started")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, handler.Shutdown(ctx), context.DeadlineExceeded)
	close(svc.release)
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, handler.Shutdown(ctx))
	handler.lifecycleMu.Lock()
	defer handler.lifecycleMu.Unlock()
	require.Zero(t, handler.activeHandlers)
	require.Empty(t, handler.connections)
}
