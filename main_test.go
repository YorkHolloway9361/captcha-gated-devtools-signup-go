package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSignupRejectsCaptchaBeforeCreatingUser(t *testing.T) {
	created := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/captcha/verify" {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]string{"code": "CAPTCHA_SCORE_TOO_LOW", "message": "captcha rejected"}})
			return
		}
		created = true
		t.Fatal("user creation must not run after a rejected captcha")
	}))
	defer server.Close()

	api := &InfraiClient{baseURL: server.URL + "/v1", key: "test-key", http: server.Client()}
	req := httptest.NewRequest(http.MethodPost, "/signup", strings.NewReader(`{"widget_record_id":"widget-1","token":"bot-token","email":"dev@example.com","password":"pass","idempotency_key":"signup-1"}`))
	req.RemoteAddr = "203.0.113.7:1234"
	response := httptest.NewRecorder()
	(&CaptchaGate{api: api}).signup(response, req)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", response.Code)
	}
	if created {
		t.Fatal("created = true, want false")
	}
	_ = context.Background()
}
