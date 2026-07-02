package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadDefault(t *testing.T) {
	t.Setenv("GATEWAY_CONFIG", "")
	t.Setenv("GATEWAY_ADDRESS", "")
	t.Setenv("GATEWAY_ADDR", "")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Server.Address != ":8080" {
		t.Fatalf("默认监听地址 = %q, want :8080", cfg.Server.Address)
	}
	if len(cfg.AI.Backends) != 2 {
		t.Fatalf("默认模型后端数量 = %d, want 2", len(cfg.AI.Backends))
	}
	if cfg.RateLimit.User.Enabled {
		t.Fatal("默认用户级限流应关闭")
	}
	if cfg.Auth.APIKey.Enabled {
		t.Fatal("默认 API Key 鉴权应关闭")
	}
	if cfg.Auth.APIKey.Header != "Authorization" {
		t.Fatalf("默认 API Key 请求头 = %q, want Authorization", cfg.Auth.APIKey.Header)
	}
}

func TestLoadFile(t *testing.T) {
	t.Setenv("GATEWAY_CONFIG", "")
	t.Setenv("GATEWAY_ADDRESS", "")
	t.Setenv("GATEWAY_ADDR", "")

	path := filepath.Join(t.TempDir(), "gateway.json")
	data := []byte(`{
  "server": {
    "address": "127.0.0.1:18080",
    "shutdown_timeout": "2s"
  },
  "observability": {
    "metrics_namespace": "test_gateway"
  }
}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("写入测试配置失败: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Server.Address != "127.0.0.1:18080" {
		t.Fatalf("监听地址 = %q", cfg.Server.Address)
	}
	if cfg.Server.ShutdownTimeout.Duration != 2*time.Second {
		t.Fatalf("关闭超时 = %s", cfg.Server.ShutdownTimeout.Duration)
	}
	if cfg.Server.ReadHeaderTimeout.Duration != 5*time.Second {
		t.Fatalf("读取请求头超时应保留默认值，实际 = %s", cfg.Server.ReadHeaderTimeout.Duration)
	}
}

func TestLoadFileUserRateLimit(t *testing.T) {
	t.Setenv("GATEWAY_CONFIG", "")
	t.Setenv("GATEWAY_ADDRESS", "")
	t.Setenv("GATEWAY_ADDR", "")

	path := filepath.Join(t.TempDir(), "gateway.json")
	data := []byte(`{
  "rate_limit": {
    "user": {
      "enabled": true,
      "identity_header": "X-Tenant-User",
      "requests_per_second": 2.5,
      "burst": 4
    }
  }
}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("写入测试配置失败: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if !cfg.RateLimit.User.Enabled {
		t.Fatal("用户级限流未启用")
	}
	if cfg.RateLimit.User.IdentityHeader != "X-Tenant-User" {
		t.Fatalf("identity_header = %q", cfg.RateLimit.User.IdentityHeader)
	}
	if cfg.RateLimit.User.RequestsPerSecond != 2.5 {
		t.Fatalf("requests_per_second = %f", cfg.RateLimit.User.RequestsPerSecond)
	}
	if cfg.RateLimit.User.Burst != 4 {
		t.Fatalf("burst = %d", cfg.RateLimit.User.Burst)
	}
}

func TestLoadFileAPIKeyAuth(t *testing.T) {
	t.Setenv("GATEWAY_CONFIG", "")
	t.Setenv("GATEWAY_ADDRESS", "")
	t.Setenv("GATEWAY_ADDR", "")

	path := filepath.Join(t.TempDir(), "gateway.json")
	data := []byte(`{
  "auth": {
    "api_key": {
      "enabled": true,
      "keys": [
        {
          "id": "dev-key",
          "key_hash": "sha256:62af8704764faf8ea82fc61ce9c4c3908b6cb97d463a634e9e587d7c885db0ef",
          "user_id": "alice",
          "tenant_id": "tenant-a",
          "scopes": ["chat:completions"]
        }
      ]
    }
  }
}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("写入测试配置失败: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if !cfg.Auth.APIKey.Enabled {
		t.Fatal("API Key 鉴权未启用")
	}
	if cfg.Auth.APIKey.Header != "Authorization" {
		t.Fatalf("header = %q, want Authorization", cfg.Auth.APIKey.Header)
	}
	if len(cfg.Auth.APIKey.Keys) != 1 {
		t.Fatalf("keys 数量 = %d, want 1", len(cfg.Auth.APIKey.Keys))
	}
	if cfg.Auth.APIKey.Keys[0].UserID != "alice" {
		t.Fatalf("user_id = %q, want alice", cfg.Auth.APIKey.Keys[0].UserID)
	}
}

func TestValidateRejectsInvalidAddress(t *testing.T) {
	cfg := Default()
	cfg.Server.Address = "8080"

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want invalid address error")
	}
}

func TestValidateRejectsDuplicateModel(t *testing.T) {
	cfg := Default()
	cfg.AI.Backends[1].Models = []string{"mock-a"}

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want duplicate model error")
	}
}

func TestValidateRejectsInvalidUserRateLimit(t *testing.T) {
	cfg := Default()
	cfg.RateLimit.User.Enabled = true
	cfg.RateLimit.User.RequestsPerSecond = 0

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want invalid user rate limit error")
	}
}

func TestValidateRejectsInvalidAPIKeyAuth(t *testing.T) {
	cfg := Default()
	cfg.Auth.APIKey.Enabled = true
	cfg.Auth.APIKey.Keys = []APIKeyCredentialConfig{
		{
			ID:      "key-a",
			KeyHash: "not-a-sha256-hash",
			UserID:  "alice",
		},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want invalid API Key auth error")
	}
}
