package config

import (
	"encoding/hex"
	"fmt"
	"strings"
)

type AuthConfig struct {
	APIKey APIKeyAuthConfig `json:"api_key"`
}

type APIKeyAuthConfig struct {
	Enabled bool                     `json:"enabled"`
	Header  string                   `json:"header"`
	Keys    []APIKeyCredentialConfig `json:"keys"`
}

type APIKeyCredentialConfig struct {
	ID       string   `json:"id"`
	KeyHash  string   `json:"key_hash"`
	UserID   string   `json:"user_id"`
	TenantID string   `json:"tenant_id,omitempty"`
	Scopes   []string `json:"scopes,omitempty"`
}

func validateAPIKeyAuth(cfg APIKeyAuthConfig) error {
	if !cfg.Enabled {
		return nil
	}

	if strings.TrimSpace(cfg.Header) == "" {
		return fmt.Errorf("auth.api_key.header 不能为空")
	}
	if strings.ContainsAny(cfg.Header, " \t\r\n:") {
		return fmt.Errorf("auth.api_key.header 包含非法字符")
	}
	if len(cfg.Keys) == 0 {
		return fmt.Errorf("auth.api_key.keys 至少需要配置一个 API Key")
	}

	ids := make(map[string]struct{}, len(cfg.Keys))
	hashes := make(map[string]struct{}, len(cfg.Keys))
	for i, key := range cfg.Keys {
		id := strings.TrimSpace(key.ID)
		if id == "" {
			return fmt.Errorf("auth.api_key.keys[%d].id 不能为空", i)
		}
		if _, ok := ids[id]; ok {
			return fmt.Errorf("auth.api_key.keys[%d].id 重复: %q", i, id)
		}
		ids[id] = struct{}{}

		if strings.TrimSpace(key.UserID) == "" {
			return fmt.Errorf("auth.api_key.keys[%d].user_id 不能为空", i)
		}

		hash, err := normalizeAPIKeyHash(key.KeyHash)
		if err != nil {
			return fmt.Errorf("auth.api_key.keys[%d].key_hash 无效: %w", i, err)
		}
		if _, ok := hashes[hash]; ok {
			return fmt.Errorf("auth.api_key.keys[%d].key_hash 重复", i)
		}
		hashes[hash] = struct{}{}

		for scopeIndex, scope := range key.Scopes {
			if strings.TrimSpace(scope) == "" {
				return fmt.Errorf("auth.api_key.keys[%d].scopes[%d] 不能为空", i, scopeIndex)
			}
		}
	}

	return nil
}

func normalizeAPIKeyHash(value string) (string, error) {
	hash := strings.TrimSpace(strings.ToLower(value))
	hash = strings.TrimPrefix(hash, "sha256:")
	if len(hash) != 64 {
		return "", fmt.Errorf("必须是 sha256:<64位hex> 或 64位hex")
	}
	if _, err := hex.DecodeString(hash); err != nil {
		return "", err
	}
	return hash, nil
}
