package auth

import (
	"net/http"
	"strings"
)

// Middleware 返回 API Key 鉴权中间件。
func (a *APIKeyAuthenticator) Middleware(next http.Handler) http.Handler {
	if a == nil {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey, ok := a.extractAPIKey(r)
		if !ok {
			writeAuthError(w, http.StatusUnauthorized, "缺少 API Key", "authentication_error")
			return
		}

		identity, ok := a.authenticate(apiKey)
		if !ok {
			writeAuthError(w, http.StatusUnauthorized, "API Key 无效", "authentication_error")
			return
		}

		next.ServeHTTP(w, r.WithContext(WithIdentity(r.Context(), identity)))
	})
}

func (a *APIKeyAuthenticator) extractAPIKey(r *http.Request) (string, bool) {
	value := strings.TrimSpace(r.Header.Get(a.header))
	if value == "" {
		return "", false
	}

	if strings.EqualFold(a.header, "Authorization") {
		fields := strings.Fields(value)
		if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") {
			return "", false
		}
		value = fields[1]
	}

	if value == "" {
		return "", false
	}
	return value, true
}
