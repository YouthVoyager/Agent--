package llm

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/agent-gateway/telemetry-gateway/internal/config"
	"github.com/agent-gateway/telemetry-gateway/internal/observability"
)

const maxRequestBodyBytes = 10 << 20

// Handler 处理 OpenAI 兼容的 LLM HTTP 请求。
type Handler struct {
	//处理器属性,包括配置以及依赖
	logger            *slog.Logger
	client            *http.Client
	backends          map[string]modelBackend
	models            []modelInfo
	metrics           *observability.Metrics
	requestTimeout    time.Duration
	firstTokenTimeout time.Duration
	fallbacks         map[string][]string
	circuitBreaker    *circuitBreaker
}

// NewHandler 创建并初始化一个 LLM HTTP 处理器。
func NewHandler(cfg config.AIConfig, logger *slog.Logger, metrics *observability.Metrics) (*Handler, error) {
	if logger == nil {
		logger = slog.Default()
	}
	cfg = normalizeAIConfig(cfg)
	//配置tcp超时
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = cfg.RequestTimeout.Duration

	handler := &Handler{
		logger:            logger,
		client:            &http.Client{Transport: transport},
		backends:          make(map[string]modelBackend),
		metrics:           metrics,
		requestTimeout:    cfg.RequestTimeout.Duration,
		firstTokenTimeout: cfg.FirstTokenTimeout.Duration,
		fallbacks:         normalizeFallbacks(cfg.Fallbacks),
	}
	//读取配置
	backendNames := make([]string, 0, len(cfg.Backends))
	for _, backendCfg := range cfg.Backends {
		backendType := normalizeBackendType(backendCfg.Type)
		backend := modelBackend{
			cfg:         backendCfg,
			backendType: backendType,
		}
		backendNames = append(backendNames, backendCfg.Name)
		if backendType != "mock" {
			backend.chatCompletionsURL = chatCompletionsURL(backendCfg.BaseURL)
		}

		for _, model := range backendCfg.Models {
			model = strings.TrimSpace(model)
			handler.backends[model] = backend
			handler.models = append(handler.models, modelInfo{
				ID:      model,
				Backend: backendCfg.Name,
			})
		}
	}
	handler.circuitBreaker = newCircuitBreaker(cfg.CircuitBreaker, backendNames, metrics)

	return handler, nil
}

// Register 将 LLM 相关路由注册到 HTTP 多路复用器。
func Register(mux *http.ServeMux, handler *Handler, chatMiddlewares ...func(http.Handler) http.Handler) {
	chatHandler := http.Handler(handler)
	for i := len(chatMiddlewares) - 1; i >= 0; i-- {
		chatHandler = chatMiddlewares[i](chatHandler)
	}

	mux.Handle("/v1/chat/completions", chatHandler)
	mux.HandleFunc("/v1/models", handler.ListModels)
}

// ServeHTTP 处理聊天补全请求并将请求路由到对应模型后端。
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	//异常检验
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeOpenAIError(w, http.StatusMethodNotAllowed, "仅支持 POST 方法", "invalid_request_error", "")
		return
	}

	rawBody, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBodyBytes))
	if err != nil {
		writeOpenAIError(w, http.StatusRequestEntityTooLarge, "请求体过大或读取失败", "invalid_request_error", "")
		return
	}

	var req chatCompletionRequest
	if err := json.Unmarshal(rawBody, &req); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "请求体必须是有效 JSON", "invalid_request_error", "")
		return
	}

	req.Model = strings.TrimSpace(req.Model)
	if req.Model == "" {
		writeOpenAIError(w, http.StatusBadRequest, "model 不能为空", "invalid_request_error", "model")
		return
	}
	if len(req.Messages) == 0 {
		writeOpenAIError(w, http.StatusBadRequest, "messages 至少需要一条消息", "invalid_request_error", "messages")
		return
	}

	if _, ok := h.backends[req.Model]; !ok {
		writeOpenAIError(w, http.StatusNotFound, fmt.Sprintf("模型 %q 未配置后端", req.Model), "invalid_request_error", "model")
		return
	}

	h.serveChatCompletionWithFallback(w, r, rawBody, req)
}

// ListModels 处理模型列表查询请求。
func (h *Handler) ListModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeOpenAIError(w, http.StatusMethodNotAllowed, "仅支持 GET 方法", "invalid_request_error", "")
		return
	}

	created := time.Now().Unix()
	data := make([]modelData, 0, len(h.models))
	for _, model := range h.models {
		data = append(data, modelData{
			ID:      model.ID,
			Object:  "model",
			Created: created,
			OwnedBy: model.Backend,
		})
	}

	writeJSON(w, http.StatusOK, modelsResponse{
		Object: "list",
		Data:   data,
	})
}
