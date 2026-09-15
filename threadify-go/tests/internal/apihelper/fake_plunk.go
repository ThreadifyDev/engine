package apihelper

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
)

type FakePlunk struct {
	URL      string
	server   *httptest.Server
	mu       sync.Mutex
	messages []CapturedEmail
}

type CapturedEmail struct {
	To      string `json:"to"`
	Body    string `json:"body"`
	Subject string `json:"subject"`
}

func (f *FakePlunk) MessagesTo(email string) []CapturedEmail {
	f.mu.Lock()
	defer f.mu.Unlock()
	var result []CapturedEmail
	for _, message := range f.messages {
		if message.To == email {
			result = append(result, message)
		}
	}
	return result
}

func StartFakePlunk() *FakePlunk {
	f := &FakePlunk{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var message CapturedEmail
		_ = json.NewDecoder(r.Body).Decode(&message)
		f.mu.Lock()
		f.messages = append(f.messages, message)
		f.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	f.URL, f.server = srv.URL, srv
	return f
}

func (f *FakePlunk) Close() {
	if f != nil && f.server != nil {
		f.server.Close()
	}
}
