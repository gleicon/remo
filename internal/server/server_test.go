package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"
)

func TestStatusHandlerAuth(t *testing.T) {
	srv := New(Config{Domain: "rempapps.site", Logger: zerolog.New(io.Discard), AdminSecret: "secret"})
	unauth := httptest.NewRecorder()
	srv.handleStatus(unauth, httptest.NewRequest(http.MethodGet, "/status", nil))
	if unauth.Result().StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401")
	}
	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	req.Header.Set("Authorization", "Bearer secret")
	recorder := httptest.NewRecorder()
	srv.handleStatus(recorder, req)
	resp := recorder.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status code %d", resp.StatusCode)
	}
	var payload statusResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.Domain != "rempapps.site" {
		t.Fatalf("unexpected domain %s", payload.Domain)
	}
}
