package engine

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

type TestResponse struct {
	StatusCode int
	Body       []byte
}

type TestUser struct {
	ID        string
	Email     string
	Token     string
	ApiKey    string
	CompanyID string
}

func doRaw(t *testing.T, method, path string, body []byte, contentType string) *TestResponse {
	req, err := http.NewRequest(method, engineApp.BaseURL+path, bytes.NewReader(body))
	require.NoError(t, err)

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := engineApp.Client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return &TestResponse{StatusCode: resp.StatusCode, Body: respBody}
}

func doJSON(t *testing.T, method, path string, body interface{}) *TestResponse {
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		require.NoError(t, err)
	}
	return doRaw(t, method, path, bodyBytes, "application/json")
}

func doWithAuth(t *testing.T, method, path string, body []byte, contentType, token string) *TestResponse {
	req, err := http.NewRequest(method, engineApp.BaseURL+path, bytes.NewReader(body))
	require.NoError(t, err)

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := engineApp.Client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return &TestResponse{StatusCode: resp.StatusCode, Body: respBody}
}

func decodeJSONBody(t *testing.T, resp *TestResponse) map[string]interface{} {
	var body map[string]interface{}
	err := json.Unmarshal(resp.Body, &body)
	require.NoError(t, err, "failed to decode JSON body: %s", string(resp.Body))
	return body
}

func dialWS(t *testing.T, path string) *websocket.Conn {
	conn, _, err := websocket.DefaultDialer.Dial(engineApp.WSURL+path, nil)
	require.NoError(t, err)
	return conn
}

func sendWSJSON(t *testing.T, conn *websocket.Conn, msg interface{}) {
	require.NoError(t, conn.WriteJSON(msg))
}

func readWSJSON(t *testing.T, conn *websocket.Conn, v interface{}) {
	require.NoError(t, conn.ReadJSON(v))
}

func readWSWithTimeout(t *testing.T, conn *websocket.Conn, timeout time.Duration) map[string]interface{} {
	done := make(chan map[string]interface{})
	errChan := make(chan error)

	go func() {
		var msg map[string]interface{}
		if err := conn.ReadJSON(&msg); err != nil {
			errChan <- err
			return
		}
		done <- msg
	}()

	select {
	case msg := <-done:
		return msg
	case err := <-errChan:
		t.Fatalf("WS read error: %v", err)
	case <-time.After(timeout):
		t.Fatal("WS read timeout")
	}
	return nil
}
