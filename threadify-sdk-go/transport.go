package threadify

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/gorilla/websocket"
)

// Transport defines the operations required for WebSocket communication.
type Transport interface {
	// Send serialises and sends a JSON message.
	Send(msg map[string]any) error
	// Recv blocks until a JSON message is received.
	Recv() (map[string]any, error)
	// Close terminates the connection.
	Close() error
}

// Dialer connects to a WebSocket endpoint and returns a Transport.
type Dialer interface {
	Dial(ctx context.Context, wsURL string) (Transport, error)
}

// --- Gorilla WebSocket implementation ---

// GorillaDialer dials a WebSocket endpoint using gorilla/websocket.
type GorillaDialer struct{}

// Dial opens a WebSocket connection.
func (d *GorillaDialer) Dial(ctx context.Context, wsURL string) (Transport, error) {
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("websocket dial: %w", err)
	}
	return &GorillaTransport{conn: conn}, nil
}

// GorillaTransport wraps a gorilla/websocket connection.
type GorillaTransport struct {
	conn *websocket.Conn
	mu   sync.Mutex // guards writes
}

// Send marshals the message as JSON and writes it.
func (t *GorillaTransport) Send(msg map[string]any) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.conn.WriteMessage(websocket.TextMessage, data)
}

// Recv reads the next message and unmarshals it from JSON.
func (t *GorillaTransport) Recv() (map[string]any, error) {
	_, data, err := t.conn.ReadMessage()
	if err != nil {
		return nil, err
	}
	var msg map[string]any
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("unmarshal message: %w", err)
	}
	return msg, nil
}

// Close closes the underlying WebSocket connection.
func (t *GorillaTransport) Close() error {
	return t.conn.Close()
}
