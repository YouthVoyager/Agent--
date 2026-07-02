package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agent-gateway/telemetry-gateway/internal/config"
)

const testKeyHash = "sha256:62af8704764faf8ea82fc61ce9c4c3908b6cb97d463a634e9e587d7c885db0ef"

func TestAPIKeyAuthenticatorAllowsValidBearerToken(t *testing.T) {
	authenticator := newTestAuthenticator(t, config.APIKeyAuthConfig{
		Header: "Authorization",
		Keys: []config.APIKeyCredentialConfig{
			{
				ID:       "key-a",
				KeyHash:  testKeyHash,
				UserID:   "alice",
				TenantID: "tenant-a",
				Scopes:   []string{"chat:completions"},
			},
		},
	})

	var gotIdentity Identity
	handler := authenticator.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ok bool
		gotIdentity, ok = IdentityFromContext(r.Context())
		if !ok {
			t.Fatal("上下文缺少调用方身份")
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	request.Header.Set("Authorization", "Bearer test-key")
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
	if gotIdentity.UserID != "alice" {
		t.Fatalf("UserID = %q, want alice", gotIdentity.UserID)
	}
	if gotIdentity.TenantID != "tenant-a" {
		t.Fatalf("TenantID = %q, want tenant-a", gotIdentity.TenantID)
	}
	if len(gotIdentity.Scopes) != 1 || gotIdentity.Scopes[0] != "chat:completions" {
		t.Fatalf("Scopes = %#v, want [chat:completions]", gotIdentity.Scopes)
	}
}

func TestAPIKeyAuthenticatorRejectsMissingKey(t *testing.T) {
	authenticator := newTestAuthenticator(t, config.APIKeyAuthConfig{
		Header: "Authorization",
		Keys: []config.APIKeyCredentialConfig{
			{
				ID:      "key-a",
				KeyHash: testKeyHash,
				UserID:  "alice",
			},
		},
	})

	calls := 0
	handler := authenticator.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusNoContent)
	}))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
	if !strings.Contains(recorder.Body.String(), "缺少 API Key") {
		t.Fatalf("响应体未说明缺少 API Key: %s", recorder.Body.String())
	}
	if calls != 0 {
		t.Fatalf("鉴权失败时仍调用下游，calls = %d", calls)
	}
}

func TestAPIKeyAuthenticatorRejectsInvalidKey(t *testing.T) {
	authenticator := newTestAuthenticator(t, config.APIKeyAuthConfig{
		Header: "Authorization",
		Keys: []config.APIKeyCredentialConfig{
			{
				ID:      "key-a",
				KeyHash: testKeyHash,
				UserID:  "alice",
			},
		},
	})

	handler := authenticator.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	request.Header.Set("Authorization", "Bearer wrong-key")
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
	if !strings.Contains(recorder.Body.String(), "API Key 无效") {
		t.Fatalf("响应体未说明 API Key 无效: %s", recorder.Body.String())
	}
}

func TestAPIKeyAuthenticatorAllowsCustomHeader(t *testing.T) {
	authenticator := newTestAuthenticator(t, config.APIKeyAuthConfig{
		Header: "X-API-Key",
		Keys: []config.APIKeyCredentialConfig{
			{
				ID:      "key-a",
				KeyHash: testKeyHash,
				UserID:  "alice",
			},
		},
	})

	handler := authenticator.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	request.Header.Set("X-API-Key", "test-key")
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
}

func newTestAuthenticator(t *testing.T, cfg config.APIKeyAuthConfig) *APIKeyAuthenticator {
	t.Helper()

	authenticator, err := NewAPIKeyAuthenticator(cfg)
	if err != nil {
		t.Fatalf("NewAPIKeyAuthenticator() error = %v", err)
	}
	return authenticator
}
