package registry

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAccountingDelegationIsBoundOneUseAndEngineOnly(t *testing.T) {
	pool := testPool(t)
	snapshot := testSnapshot()
	snapshot.Entitlements.InputRequestsPerSecond = 0
	r := &Runtime{cfg: Config{CompanyID: "company", InstallationID: newID(), LicenseKey: "license"}, accountID: snapshot.AccountID, pool: pool, snapshot: snapshot, verified: time.Now()}
	if err := r.initializeStore(context.Background()); err != nil {
		t.Fatal(err)
	}
	var calls int
	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		data, err := io.ReadAll(req.Body)
		if err != nil {
			t.Error(err)
		}
		if string(data) != "payload" {
			t.Errorf("unexpected payload %q", data)
		}
		calls++
		w.WriteHeader(200)
	})
	signed := func() *http.Request {
		req, err := http.NewRequest("POST", "http://engine.test/graphql?test=1", strings.NewReader("payload"))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer user-token")
		req.Header.Set("Content-Type", "application/json")
		if err = r.SignRequest(req); err != nil {
			t.Fatal(err)
		}
		return req
	}
	request := signed()
	proof := request.Header.Get(AccountingDelegationHeader)
	gatewayResponse := httptest.NewRecorder()
	r.Wrap(handler).ServeHTTP(gatewayResponse, request)
	if gatewayResponse.Code != 429 || calls != 0 {
		t.Fatal("gateway accepted caller-supplied exemption")
	}
	request = signed()
	proof = request.Header.Get(AccountingDelegationHeader)
	response := httptest.NewRecorder()
	r.WrapEngine(handler).ServeHTTP(response, request)
	if response.Code != 200 || calls != 1 {
		t.Fatalf("delegated hop was not accepted: status%d calls%d", response.Code, calls)
	}
	// Failed proofs do not exempt callers from the normal request quota.
	bad := signed()
	bad.Header.Set(AccountingDelegationHeader, "invalid")
	badResponse := httptest.NewRecorder()
	r.WrapEngine(handler).ServeHTTP(badResponse, bad)
	if badResponse.Code != 429 {
		t.Fatalf("invalid proof bypassed rate quota: %d", badResponse.Code)
	}
	r.snapshot.Entitlements.InputRequestsPerSecond = -1
	replay := signed()
	replay.Header.Set(AccountingDelegationHeader, proof)
	response = httptest.NewRecorder()
	r.WrapEngine(handler).ServeHTTP(response, replay)
	if response.Code != 401 || calls != 1 {
		t.Fatal("accounting proof replay was accepted")
	}
	for _, field := range []string{"body", "authorization", "path", "content-type", "signature"} {
		t.Run(field, func(t *testing.T) {
			req := signed()
			switch field {
			case "body":
				req.Body = io.NopCloser(strings.NewReader("mutated"))
			case "authorization":
				req.Header.Set("Authorization", "Bearer someone-else")
			case "path":
				req.URL.Path = "/v1/contracts"
			case "content-type":
				req.Header.Set("Content-Type", "text/plain")
			case "signature":
				req.Header.Set(AccountingDelegationHeader, req.Header.Get(AccountingDelegationHeader)+"0")
			}
			w := httptest.NewRecorder()
			r.WrapEngine(handler).ServeHTTP(w, req)
			if w.Code != 401 || calls != 1 {
				t.Fatalf("tampered %s accepted status%d", field, w.Code)
			}
		})
	}
}

func TestAccountingDelegationRejectsStreamingBody(t *testing.T) {
	snapshot := testSnapshot()
	r := &Runtime{snapshot: snapshot, verified: time.Now()}
	req := httptest.NewRequest("POST", "/graphql", io.NopCloser(strings.NewReader("payload")))
	if err := r.SignRequest(req); err == nil {
		t.Fatal("unreplayable body was signed")
	}
}
