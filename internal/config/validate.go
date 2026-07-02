package config

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

func (c Config) Validate() error {
	if strings.TrimSpace(c.Server.Address) == "" {
		return fmt.Errorf("server.address 不能为空")
	}

	_, port, err := net.SplitHostPort(c.Server.Address)
	if err != nil {
		return fmt.Errorf("server.address 必须是 host:port 或 :port 格式: %w", err)
	}

	portNum, err := strconv.Atoi(port)
	if err != nil || portNum <= 0 || portNum > 65535 {
		return fmt.Errorf("server.address 端口无效: %q", port)
	}

	if c.Server.ReadHeaderTimeout.Duration <= 0 {
		return fmt.Errorf("server.read_header_timeout 必须大于 0")
	}
	if c.Server.ShutdownTimeout.Duration <= 0 {
		return fmt.Errorf("server.shutdown_timeout 必须大于 0")
	}
	if strings.TrimSpace(c.Observability.MetricsNamespace) == "" {
		return fmt.Errorf("observability.metrics_namespace 不能为空")
	}
	if err := validateAPIKeyAuth(c.Auth.APIKey); err != nil {
		return err
	}
	if c.RateLimit.User.Enabled {
		if strings.TrimSpace(c.RateLimit.User.IdentityHeader) == "" {
			return fmt.Errorf("rate_limit.user.identity_header 不能为空")
		}
		if c.RateLimit.User.RequestsPerSecond <= 0 {
			return fmt.Errorf("rate_limit.user.requests_per_second 必须大于 0")
		}
		if c.RateLimit.User.Burst <= 0 {
			return fmt.Errorf("rate_limit.user.burst 必须大于 0")
		}
	}
	if c.RateLimit.Concurrency.Enabled && c.RateLimit.Concurrency.MaxInFlight <= 0 {
		return fmt.Errorf("rate_limit.concurrency.max_in_flight 必须大于 0")
	}
	if c.AI.RequestTimeout.Duration <= 0 {
		return fmt.Errorf("ai.request_timeout 必须大于 0")
	}
	if len(c.AI.Backends) < 2 {
		return fmt.Errorf("ai.backends 至少需要配置 2 个模型后端")
	}

	backendNames := make(map[string]struct{}, len(c.AI.Backends))
	models := make(map[string]string)
	for i, backend := range c.AI.Backends {
		name := strings.TrimSpace(backend.Name)
		if name == "" {
			return fmt.Errorf("ai.backends[%d].name 不能为空", i)
		}
		if _, ok := backendNames[name]; ok {
			return fmt.Errorf("ai.backends[%d].name 重复: %q", i, name)
		}
		backendNames[name] = struct{}{}

		backendType := strings.TrimSpace(strings.ToLower(backend.Type))
		switch backendType {
		case "mock":
		case "openai", "openai_compatible", "openai-compatible":
			baseURL := strings.TrimSpace(backend.BaseURL)
			if baseURL == "" {
				return fmt.Errorf("ai.backends[%d].base_url 不能为空", i)
			}
			parsed, err := url.Parse(baseURL)
			if err != nil || parsed.Scheme == "" || parsed.Host == "" {
				return fmt.Errorf("ai.backends[%d].base_url 必须是有效的绝对 URL", i)
			}
			if parsed.Scheme != "http" && parsed.Scheme != "https" {
				return fmt.Errorf("ai.backends[%d].base_url 仅支持 http 或 https", i)
			}
		default:
			return fmt.Errorf("ai.backends[%d].type 不支持: %q", i, backend.Type)
		}

		if len(backend.Models) == 0 {
			return fmt.Errorf("ai.backends[%d].models 至少需要一个模型名", i)
		}
		for modelIndex, model := range backend.Models {
			model = strings.TrimSpace(model)
			if model == "" {
				return fmt.Errorf("ai.backends[%d].models[%d] 不能为空", i, modelIndex)
			}
			if owner, ok := models[model]; ok {
				return fmt.Errorf("模型 %q 同时配置到后端 %q 和 %q", model, owner, name)
			}
			models[model] = name
		}
	}

	return nil
}
