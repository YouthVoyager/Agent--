package llm

import "strings"
//规范配置格式
func normalizeBackendType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "openai", "openai_compatible", "openai-compatible":
		return "openai_compatible"
	default:
		return "mock"
	}
}

func chatCompletionsURL(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if strings.HasSuffix(base, "/chat/completions") {
		return base
	}
	if strings.HasSuffix(base, "/v1") {
		return base + "/chat/completions"
	}
	return base + "/v1/chat/completions"
}
