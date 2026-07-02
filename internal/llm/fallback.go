package llm

import (
	"strings"
	"time"

	"github.com/agent-gateway/telemetry-gateway/internal/config"
)

func normalizeAIConfig(cfg config.AIConfig) config.AIConfig {
	//加载默认配置
	if cfg.RequestTimeout.Duration <= 0 {
		cfg.RequestTimeout = config.Duration{Duration: 30 * time.Second}
	}
	if cfg.FirstTokenTimeout.Duration <= 0 {
		cfg.FirstTokenTimeout = cfg.RequestTimeout
	}
	if cfg.CircuitBreaker.FailureThreshold <= 0 {
		cfg.CircuitBreaker.FailureThreshold = 3
	}
	if cfg.CircuitBreaker.OpenTimeout.Duration <= 0 {
		cfg.CircuitBreaker.OpenTimeout = config.Duration{Duration: 30 * time.Second}
	}
	if cfg.CircuitBreaker.HalfOpenMaxRequests <= 0 {
		cfg.CircuitBreaker.HalfOpenMaxRequests = 1
	}
	return cfg
}

func normalizeFallbacks(fallbacks map[string][]string) map[string][]string {
	normalized := make(map[string][]string, len(fallbacks))
	for source, targets := range fallbacks {
		source = strings.TrimSpace(source)
		if source == "" {
			continue
		}

		seen := make(map[string]struct{}, len(targets))
		for _, target := range targets {
			target = strings.TrimSpace(target)
			if target == "" {
				continue
			}
			if _, ok := seen[target]; ok {
				continue
			}
			seen[target] = struct{}{}
			normalized[source] = append(normalized[source], target)
		}
	}
	return normalized
}

func (h *Handler) candidateModels(model string) []string {
	//列出候选模型表
	candidates := make([]string, 0, 1+len(h.fallbacks[model]))
	//防止模型链循环
	seen := make(map[string]struct{})

	var visit func(string)
	visit = func(current string) {
		if _, ok := seen[current]; ok {
			return
		}
		if _, ok := h.backends[current]; !ok {
			return
		}
		seen[current] = struct{}{}
		candidates = append(candidates, current)
		for _, fallback := range h.fallbacks[current] {
			visit(fallback)
		}
	}

	visit(model)
	return candidates
}
