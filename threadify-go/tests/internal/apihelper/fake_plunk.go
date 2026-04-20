package apihelper

import (
	"net/http"
	"net/http/httptest"
)

type FakePlunk struct {
	URL    string
	server *httptest.Server
}

func StartFakePlunk() *FakePlunk {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	return &FakePlunk{URL: srv.URL, server: srv}
}

func (f *FakePlunk) Close() {
	if f != nil && f.server != nil {
		f.server.Close()
	}
}
