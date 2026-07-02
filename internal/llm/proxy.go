package llm

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/agent-gateway/telemetry-gateway/internal/config"
)

func (h *Handler) proxyOpenAICompatible(w http.ResponseWriter, r *http.Request, rawBody []byte, req chatCompletionRequest, backend modelBackend) {
	//开始计时,测量首Token及请求延迟
	start := time.Now()
	result := requestResultFailure
	defer func() {
		h.observeBackendRequest(backend.cfg.Name, result, time.Since(start))
	}()
	//构造http请求
	upstreamReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, backend.chatCompletionsURL, bytes.NewReader(rawBody))
	if err != nil {
		writeOpenAIError(w, http.StatusBadGateway, "创建上游请求失败", "server_error", "")
		return
	}
	//设置请求头
	upstreamReq.Header.Set("Content-Type", "application/json")
	if accept := r.Header.Get("Accept"); accept != "" {
		upstreamReq.Header.Set("Accept", accept)
	}
	if organization := r.Header.Get("OpenAI-Organization"); organization != "" {
		upstreamReq.Header.Set("OpenAI-Organization", organization)
	}
	if project := r.Header.Get("OpenAI-Project"); project != "" {
		upstreamReq.Header.Set("OpenAI-Project", project)
	}

	if apiKey := resolveAPIKey(backend.cfg); apiKey != "" {
		upstreamReq.Header.Set("Authorization", "Bearer "+apiKey)
	} else if authorization := r.Header.Get("Authorization"); authorization != "" {
		upstreamReq.Header.Set("Authorization", authorization)
	}
	//发送请求
	resp, err := h.client.Do(upstreamReq)
	if err != nil {
		h.logger.Warn("模型后端请求失败", "backend", backend.cfg.Name, "error", err)
		writeOpenAIError(w, http.StatusBadGateway, "模型后端请求失败", "server_error", "")
		return
	}
	defer resp.Body.Close()
	//检测是否支持流式请求
	if req.Stream {
		if _, ok := w.(http.Flusher); !ok {
			writeOpenAIError(w, http.StatusInternalServerError, "当前 HTTP writer 不支持流式刷新", "server_error", "")
			return
		}
	}
	//判断请求是否成功
	result = requestResultFromStatus(resp.StatusCode)
	//复制响应头
	copyResponseHeaders(w.Header(), resp.Header)
	if req.Stream && resp.Header.Get("Content-Type") == "" && resp.StatusCode < 400 {
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	}
	w.WriteHeader(resp.StatusCode)

	if req.Stream {
		//将流式请求转发
		if ok := streamCopy(w, resp.Body, h.metrics, backend.cfg.Name, start); !ok && result == requestResultSuccess {
			result = requestResultFailure
		}
		return
	}
	if _, err := io.Copy(w, resp.Body); err != nil && result == requestResultSuccess {
		result = requestResultFailure
	}
}

func copyResponseHeaders(dst, src http.Header) {
	for key, values := range src {
		if isHopByHopHeader(key) {
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func isHopByHopHeader(key string) bool {
	switch strings.ToLower(key) {
	case "connection", "keep-alive", "proxy-authenticate", "proxy-authorization",
		"te", "trailer", "transfer-encoding", "upgrade", "content-length":
		return true
	default:
		return false
	}
}

func resolveAPIKey(cfg config.ModelBackendConfig) string {
	if cfg.APIKeyEnv != "" {
		if value := os.Getenv(cfg.APIKeyEnv); value != "" {
			return value
		}
	}
	return cfg.APIKey
}
