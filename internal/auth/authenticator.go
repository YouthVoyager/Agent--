package auth

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"

	"github.com/agent-gateway/telemetry-gateway/internal/config"
)

type apiKeyCredential struct {
	hash     []byte
	identity Identity
}

// APIKeyAuthenticator 校验请求中的 API Key 并解析调用方身份。
type APIKeyAuthenticator struct {
	header      string
	credentials []apiKeyCredential
}

// NewAPIKeyAuthenticator 根据配置创建 API Key 鉴权器。
func NewAPIKeyAuthenticator(cfg config.APIKeyAuthConfig) (*APIKeyAuthenticator, error) {
	header := strings.TrimSpace(cfg.Header)
	if header == "" {
		return nil, fmt.Errorf("auth.api_key.header 不能为空")
	}
	if strings.ContainsAny(header, " \t\r\n:") {
		return nil, fmt.Errorf("auth.api_key.header 包含非法字符")
	}
	if len(cfg.Keys) == 0 {
		return nil, fmt.Errorf("auth.api_key.keys 至少需要配置一个 API Key")
	}

	credentials := make([]apiKeyCredential, 0, len(cfg.Keys))
	for i, key := range cfg.Keys {
		hash, err := parseAPIKeyHash(key.KeyHash)
		if err != nil {
			return nil, fmt.Errorf("解析 auth.api_key.keys[%d].key_hash: %w", i, err)
		}

		credentials = append(credentials, apiKeyCredential{
			hash: hash,
			identity: Identity{
				KeyID:    strings.TrimSpace(key.ID),
				UserID:   strings.TrimSpace(key.UserID),
				TenantID: strings.TrimSpace(key.TenantID),
				Scopes:   normalizeScopes(key.Scopes),
			},
		})
	}

	return &APIKeyAuthenticator{
		header:      http.CanonicalHeaderKey(header),
		credentials: credentials,
	}, nil
}

func (a *APIKeyAuthenticator) authenticate(apiKey string) (Identity, bool) {
	digest := sha256.Sum256([]byte(apiKey))

	var matchedIdentity Identity
	matched := false
	for _, credential := range a.credentials {
		if subtle.ConstantTimeCompare(digest[:], credential.hash) == 1 {
			matchedIdentity = credential.identity
			matched = true
		}
	}
	if !matched {
		return Identity{}, false
	}

	return cloneIdentity(matchedIdentity), true
}

func parseAPIKeyHash(value string) ([]byte, error) {
	hash := strings.TrimSpace(strings.ToLower(value))
	hash = strings.TrimPrefix(hash, "sha256:")
	if len(hash) != 64 {
		return nil, fmt.Errorf("必须是 sha256:<64位hex> 或 64位hex")
	}
	return hex.DecodeString(hash)
}

func normalizeScopes(scopes []string) []string {
	normalized := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope != "" {
			normalized = append(normalized, scope)
		}
	}
	return normalized
}
