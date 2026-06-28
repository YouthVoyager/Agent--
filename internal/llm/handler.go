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

type Handler struct {
	logger   *slog.Logger
	client   *http.Client
	backends map[string]modelBackend
	models   []modelInfo
	metrics  *observability.Metrics
}

func NewHandler(cfg config.AIConfig, logger *slog.Logger, metrics *observability.Metrics) (*Handler, error) {
	if logger == nil {
		logger = slog.Default()
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = cfg.RequestTimeout.Duration

	handler := &Handler{
		logger:   logger,
		client:   &http.Client{Transport: transport},
		backends: make(map[string]modelBackend),
		metrics:  metrics,
	}

	for _, backendCfg := range cfg.Backends {
		backendType := normalizeBackendType(backendCfg.Type)
		backend := modelBackend{
			cfg:         backendCfg,
			backendType: backendType,
		}
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

	return handler, nil
}

func Register(mux *http.ServeMux, handler *Handler) {
	mux.Handle("/v1/chat/completions", handler)
	mux.HandleFunc("/v1/models", handler.ListModels)
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

	backend, ok := h.backends[req.Model]
	if !ok {
		writeOpenAIError(w, http.StatusNotFound, fmt.Sprintf("模型 %q 未配置后端", req.Model), "invalid_request_error", "model")
		return
	}

	if backend.backendType == "mock" {
		h.serveMock(w, r, req, backend)
		return
	}

	h.proxyOpenAICompatible(w, r, rawBody, req, backend)
}

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
