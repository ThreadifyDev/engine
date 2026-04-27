package enginetest

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

type HTTPClient struct {
	BaseURL string
	WSURL   string
	Client  *http.Client
}

func NewHTTPClient(baseURL, wsURL string, client *http.Client) *HTTPClient {
	return &HTTPClient{
		BaseURL: baseURL,
		WSURL:   wsURL,
		Client:  client,
	}
}

func (c *HTTPClient) DoRaw(t *testing.T, method, path string, body []byte, contentType string) *TestResponse {
	t.Helper()
	req, err := http.NewRequest(method, c.BaseURL+path, bytes.NewReader(body))
	require.NoError(t, err)

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := c.Client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return &TestResponse{StatusCode: resp.StatusCode, Body: respBody}
}

func (c *HTTPClient) DoJSON(t *testing.T, method, path string, body interface{}) *TestResponse {
	t.Helper()
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		require.NoError(t, err)
	}
	return c.DoRaw(t, method, path, bodyBytes, "application/json")
}

func (c *HTTPClient) DoWithAuth(t *testing.T, method, path string, body []byte, contentType, token string) *TestResponse {
	t.Helper()
	req, err := http.NewRequest(method, c.BaseURL+path, bytes.NewReader(body))
	require.NoError(t, err)

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.Client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return &TestResponse{StatusCode: resp.StatusCode, Body: respBody}
}

func DecodeJSONBody(t *testing.T, resp *TestResponse) map[string]interface{} {
	t.Helper()
	var body map[string]interface{}
	err := json.Unmarshal(resp.Body, &body)
	require.NoError(t, err, "failed to decode JSON body: %s", string(resp.Body))
	return body
}

func (c *HTTPClient) DialWS(t *testing.T, path string) *websocket.Conn {
	t.Helper()
	conn, _, err := websocket.DefaultDialer.Dial(c.WSURL+path, nil)
	require.NoError(t, err)
	return conn
}

func SendWSJSON(t *testing.T, conn *websocket.Conn, msg interface{}) {
	t.Helper()
	require.NoError(t, conn.WriteJSON(msg))
}

func ReadWSWithTimeout(t *testing.T, conn *websocket.Conn, timeout time.Duration) map[string]interface{} {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	defer conn.SetReadDeadline(time.Time{})

	var msg map[string]interface{}
	if err := conn.ReadJSON(&msg); err != nil {
		t.Fatalf("WS read error: %v", err)
	}
	return msg
}

func ReadWSUntil(t *testing.T, conn *websocket.Conn, timeout time.Duration, predicate func(map[string]interface{}) bool) map[string]interface{} {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		msg := ReadWSWithTimeout(t, conn, time.Until(deadline))
		if predicate == nil || predicate(msg) {
			return msg
		}
	}
	t.Fatal("WS read timeout")
	return nil
}

func ReadWSAction(t *testing.T, conn *websocket.Conn, action string, timeout time.Duration) map[string]interface{} {
	t.Helper()
	return ReadWSUntil(t, conn, timeout, func(msg map[string]interface{}) bool {
		return msg["action"] == action
	})
}
