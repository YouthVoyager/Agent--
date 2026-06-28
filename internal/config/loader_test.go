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

func TestValidateRejectsInvalidAddress(t *testing.T) {
	cfg := Default()
	cfg.Server.Address = "8080"

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want invalid address error")
	}
}
