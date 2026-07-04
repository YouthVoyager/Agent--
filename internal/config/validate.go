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
	if err := validateTokenUsage(c.TokenUsage); err != nil {
		return err
	}
	if c.AI.RequestTimeout.Duration <= 0 {
		return fmt.Errorf("ai.request_timeout 必须大于 0")
	}
	if c.AI.FirstTokenTimeout.Duration <= 0 {
		return fmt.Errorf("ai.first_token_timeout 必须大于 0")
	}
	if err := validateCircuitBreaker(c.AI.CircuitBreaker); err != nil {
		return err
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

	if err := validateFallbacks(c.AI.Fallbacks, models); err != nil {
		return err
	}

	return nil
}

func validateTokenUsage(cfg TokenUsageConfig) error {
	if !cfg.Enabled {
		return nil
	}
	if strings.TrimSpace(cfg.IdentityHeader) == "" {
		return fmt.Errorf("token_usage.identity_header 不能为空")
	}
	if cfg.Window.Duration <= 0 {
		return fmt.Errorf("token_usage.window 必须大于 0")
	}
	if cfg.DefaultBudgetTokens <= 0 {
		return fmt.Errorf("token_usage.default_budget_tokens 必须大于 0")
	}
	if cfg.DefaultMaxCompletionTokens < 0 {
		return fmt.Errorf("token_usage.default_max_completion_tokens 不能小于 0")
	}
	for identity, budget := range cfg.UserBudgets {
		if strings.TrimSpace(identity) == "" {
			return fmt.Errorf("token_usage.user_budgets 包含空用户标识")
		}
		if budget <= 0 {
			return fmt.Errorf("token_usage.user_budgets[%q] 必须大于 0", identity)
		}
	}
	return nil
}

func validateCircuitBreaker(cfg CircuitBreakerConfig) error {
	if cfg.FailureThreshold <= 0 {
		return fmt.Errorf("ai.circuit_breaker.failure_threshold 必须大于 0")
	}
	if cfg.OpenTimeout.Duration <= 0 {
		return fmt.Errorf("ai.circuit_breaker.open_timeout 必须大于 0")
	}
	if cfg.HalfOpenMaxRequests <= 0 {
		return fmt.Errorf("ai.circuit_breaker.half_open_max_requests 必须大于 0")
	}
	return nil
}

func validateFallbacks(fallbacks map[string][]string, models map[string]string) error {
	normalized := make(map[string][]string, len(fallbacks))
	for source, targets := range fallbacks {
		source = strings.TrimSpace(source)
		if source == "" {
			return fmt.Errorf("ai.fallbacks 包含空模型名")
		}
		if _, ok := models[source]; !ok {
			return fmt.Errorf("ai.fallbacks[%q] 未配置对应模型", source)
		}

		for targetIndex, target := range targets {
			target = strings.TrimSpace(target)
			if target == "" {
				return fmt.Errorf("ai.fallbacks[%q][%d] 不能为空", source, targetIndex)
			}
			if _, ok := models[target]; !ok {
				return fmt.Errorf("ai.fallbacks[%q][%d] 指向未配置模型 %q", source, targetIndex, target)
			}
			if target == source {
				return fmt.Errorf("ai.fallbacks[%q] 不能指向自身", source)
			}
			normalized[source] = append(normalized[source], target)
		}
	}

	visiting := make(map[string]bool, len(normalized))
	visited := make(map[string]bool, len(normalized))
	var visit func(string) error
	visit = func(model string) error {
		if visiting[model] {
			return fmt.Errorf("ai.fallbacks 存在环路，涉及模型 %q", model)
		}
		if visited[model] {
			return nil
		}

		visiting[model] = true
		for _, target := range normalized[model] {
			if err := visit(target); err != nil {
				return err
			}
		}
		visiting[model] = false
		visited[model] = true
		return nil
	}

	for source := range normalized {
		if err := visit(source); err != nil {
			return err
		}
	}
	return nil
}
