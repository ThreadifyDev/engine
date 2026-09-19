package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgxpool"
)

// prepareGherkinSmoke exercises the real contract endpoints and returns a check
// that reloads the persisted rules after the standalone binary has restarted.
func prepareGherkinSmoke(t *testing.T, baseURL, apiKey string, pool *pgxpool.Pool) func() {
	t.Helper()
	contractName := fmt.Sprintf("gherkin_smoke_%d", time.Now().UnixNano())
	source := `Feature: ` + contractName + `
Version: 1
Description: Live Gherkin content validation.
Rule: A valid charge
 When step "charge" is submitted
 Then owner must be "processor"
 And content "amount" must be a number greater than 0
 And content "currency" must be one of "GBP", "USD"
 And content "reference" must match regex "^PAY-[0-9]{8}$"
 And this step is an entry point
 And this step is terminal
`
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Timeout: 10 * time.Second, Jar: jar}
	origin, err := url.Parse(baseURL)
	if err != nil {
		t.Fatal(err)
	}
	exchangeBody, err := json.Marshal(map[string]string{"api_key": apiKey})
	if err != nil {
		t.Fatal(err)
	}
	exchange, err := http.NewRequest("POST", baseURL+"/auth/api-key/exchange", strings.NewReader(string(exchangeBody)))
	if err != nil {
		t.Fatal(err)
	}
	exchange.Header.Set("Origin", baseURL)
	exchange.Header.Set("Content-Type", "application/json")
	exchanged, err := client.Do(exchange)
	if err != nil {
		t.Fatal(err)
	}
	exchangeResult, err := io.ReadAll(exchanged.Body)
	exchanged.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if exchanged.StatusCode != http.StatusOK {
		t.Fatalf("session exchange: %d %s", exchanged.StatusCode, exchangeResult)
	}
	request := func(method, path, body string, want int) map[string]any {
		t.Helper()
		req, err := http.NewRequest(method, baseURL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Origin", baseURL)
		for _, cookie := range jar.Cookies(origin) {
			if cookie.Name == "threadify_csrf_dev" {
				req.Header.Set("X-Threadify-CSRF", cookie.Value)
			}
		}
		req.Header.Set("Content-Type", "text/plain")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != want {
			t.Fatalf("%s %s: status %d want %d: %s", method, path, resp.StatusCode, want, data)
		}
		var value map[string]any
		if err := json.Unmarshal(data, &value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	preview := request("POST", "/v1/contracts/preview", source, 200)
	if preview["valid"] != true {
		t.Fatalf("preview: %v", preview)
	}
	invalid := request("POST", "/v1/contracts/preview", source+" And content must be meaningful\n", 200)
	if invalid["valid"] != false {
		t.Fatal("unsupported predicate accepted")
	}
	request("POST", "/v1/contracts", source+" And content must be meaningful\n", 400)
	invalidRegex := strings.Replace(source, "^PAY-[0-9]{8}$", "[", 1)
	if request("POST", "/v1/contracts/preview", invalidRegex, 200)["valid"] != false {
		t.Fatal("malformed regex passed preview")
	}
	request("POST", "/v1/contracts", invalidRegex, 400)
	created := request("POST", "/v1/contracts", source, 200)
	id := created["contract"].(map[string]any)["id"].(string)
	request("PUT", "/v1/contracts/"+id, strings.Replace(invalidRegex, "Version: 1", "Version: 2", 1), 400)
	updated := strings.Replace(strings.Replace(source, "Version: 1", "Version: 2", 1), "greater than 0", "greater than 10", 1)
	request("PUT", "/v1/contracts/"+id, updated, 200)
	request("PUT", "/v1/contracts/"+id, updated, 400)
	verifyWaitAfterRestart := runWaitSDKSmoke(t, baseURL, apiKey, request)
	verifyReferences := prepareCrossStepSmoke(t, baseURL, apiKey, pool, request)
	return func() {
		t.Helper()
		verifyWaitAfterRestart()
		verifyReferences()
		value := request("GET", "/v1/contracts/"+id, "", 200)["contractVersion"].(map[string]any)
		if value["source"] != updated || value["sourceFormat"] != "gherkin" || value["version"] != float64(2) {
			t.Fatalf("source/version lost after restart: %v", value)
		}
		ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(baseURL, "http")+"/threads", nil)
		if err != nil {
			t.Fatal(err)
		}
		defer ws.Close()
		send := func(action string, payload map[string]any, want string) map[string]any {
			t.Helper()
			payload["action"] = action
			if err := ws.WriteJSON(payload); err != nil {
				t.Fatal(err)
			}
			ws.SetReadDeadline(time.Now().Add(10 * time.Second))
			for {
				var result map[string]any
				if err := ws.ReadJSON(&result); err != nil {
					t.Fatal(err)
				}
				if result["action"] != action {
					continue
				}
				if result["status"] != want {
					t.Fatalf("%s: wanted %s, got %v", action, want, result)
				}
				return result
			}
		}
		send("connect", map[string]any{"apiKey": apiKey, "serviceName": "gherkin-smoke"}, "success")
		thread := send("startThread", map[string]any{"contractName": contractName + ":2", "role": "processor"}, "success")["threadId"]
		for i, tc := range []struct{ amount, currency, reference, want string }{
			{"5", "GBP", "PAY-12345678", "error"},
			{"15", "EUR", "PAY-12345678", "error"},
			{"NaN", "GBP", "PAY-12345678", "error"},
			{"15", "GBP", "invalid", "error"},
			{"15", "GBP", "", "error"},
			{"15", "GBP", "PAY-12345678", "success"},
		} {
			now := time.Now().UTC().Format(time.RFC3339Nano)
			content := map[string]string{"amount": tc.amount, "currency": tc.currency}
			if tc.reference != "" {
				content["reference"] = tc.reference
			}
			result := send("recordThreadEvent", map[string]any{"threadId": thread, "stepName": "charge", "status": "success", "startedAt": now, "finishedAt": now, "idempotencyKey": fmt.Sprintf("content-%d", i), "context": content}, tc.want)
			if tc.want == "error" && !strings.Contains(fmt.Sprint(result["message"]), "content field") {
				t.Fatalf("expected content rejection: %v", result)
			}
		}
	}
}
