package llm

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/agent-gateway/telemetry-gateway/internal/config"
	"github.com/agent-gateway/telemetry-gateway/internal/observability"
)

const maxRequestBodyBytes = 10 << 20

type Handler struct {
	logger   *slog.Logger
	client   *http.Client
	backends map[string]modelBackend
	models   []modelInfo
	metrics *observability.Metrics
}

type modelBackend struct {
	cfg                config.ModelBackendConfig
	backendType        string
	chatCompletionsURL string
}

type modelInfo struct {
	ID      string
	Backend string
}

type chatCompletionRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream,omitempty"`
}

type chatMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type chatCompletionResponse struct {
	ID      string       `json:"id"`
	Object  string       `json:"object"`
	Created int64        `json:"created"`
	Model   string       `json:"model"`
	Choices []chatChoice `json:"choices"`
	Usage   usage        `json:"usage"`
}

type chatChoice struct {
	Index        int              `json:"index"`
	Message      assistantMessage `json:"message"`
	FinishReason string           `json:"finish_reason"`
}

type assistantMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type chatCompletionChunk struct {
	ID      string        `json:"id"`
	Object  string        `json:"object"`
	Created int64         `json:"created"`
	Model   string        `json:"model"`
	Choices []chunkChoice `json:"choices"`
}

type chunkChoice struct {
	Index        int        `json:"index"`
	Delta        chunkDelta `json:"delta"`
	FinishReason *string    `json:"finish_reason"`
}

type chunkDelta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

type modelsResponse struct {
	Object string      `json:"object"`
	Data   []modelData `json:"data"`
}

type modelData struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

type errorResponse struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Message string  `json:"message"`
	Type    string  `json:"type"`
	Param   *string `json:"param"`
	Code    *string `json:"code"`
}

func NewHandler(cfg config.AIConfig, logger *slog.Logger,metrics *observability.Metrics) (*Handler, error) {
	//初始化ai输出处理器,包括初始化日志,客户端超时设置,配置模型配置
	if logger == nil {
		logger = slog.Default()
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = cfg.RequestTimeout.Duration

	handler := &Handler{
		logger:   logger,
		client:   &http.Client{Transport: transport},
		backends: make(map[string]modelBackend),
		metrics: metrics,
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
	//注册路由,一个是聊天api接口,一个是模型查询接口
	mux.Handle("/v1/chat/completions", handler)
	mux.HandleFunc("/v1/models", handler.ListModels)
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	//接受并解析请求体,并调用客户端获取ai回复
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

func (h *Handler) serveMock(w http.ResponseWriter, r *http.Request, req chatCompletionRequest, backend modelBackend) {
	if req.Stream {
		h.streamMock(w, r, req, backend)
		return
	}

	content := mockContent(req, backend)
	promptTokens := estimatePromptTokens(req.Messages)
	completionTokens := estimateTokens(content)

	writeJSON(w, http.StatusOK, chatCompletionResponse{
		ID:      newID("chatcmpl"),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []chatChoice{
			{
				Index: 0,
				Message: assistantMessage{
					Role:    "assistant",
					Content: content,
				},
				FinishReason: "stop",
			},
		},
		Usage: usage{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      promptTokens + completionTokens,
		},
	})
}

func (h *Handler) streamMock(w http.ResponseWriter, r *http.Request, req chatCompletionRequest, backend modelBackend) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeOpenAIError(w, http.StatusInternalServerError, "当前 HTTP writer 不支持流式刷新", "server_error", "")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	id := newID("chatcmpl")
	created := time.Now().Unix()
	content := mockContent(req, backend)

	if !writeSSE(w, flusher, chatCompletionChunk{
		ID:      id,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   req.Model,
		Choices: []chunkChoice{
			{
				Index:        0,
				Delta:        chunkDelta{Role: "assistant"},
				FinishReason: nil,
			},
		},
	}) {
		return
	}

	for _, piece := range splitText(content, 12) {
		select {
		case <-r.Context().Done():
			return
		default:
		}

		if !writeSSE(w, flusher, chatCompletionChunk{
			ID:      id,
			Object:  "chat.completion.chunk",
			Created: created,
			Model:   req.Model,
			Choices: []chunkChoice{
				{
					Index:        0,
					Delta:        chunkDelta{Content: piece},
					FinishReason: nil,
				},
			},
		}) {
			return
		}
	}

	finishReason := "stop"
	if !writeSSE(w, flusher, chatCompletionChunk{
		ID:      id,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   req.Model,
		Choices: []chunkChoice{
			{
				Index:        0,
				Delta:        chunkDelta{},
				FinishReason: &finishReason,
			},
		},
	}) {
		return
	}

	_, _ = io.WriteString(w, "data: [DONE]\n\n")
	flusher.Flush()
}

func (h *Handler) proxyOpenAICompatible(w http.ResponseWriter, r *http.Request, rawBody []byte, req chatCompletionRequest, backend modelBackend) {
	status := "success"
	upstreamReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, backend.chatCompletionsURL, bytes.NewReader(rawBody))
	if err != nil {
		status = "false"
		writeOpenAIError(w, http.StatusBadGateway, "创建上游请求失败", "server_error", "")
		return
	}

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

	resp, err := h.client.Do(upstreamReq)
	if err != nil {
		status = "false"
		h.logger.Warn("模型后端请求失败", "backend", backend.cfg.Name, "error", err)
		writeOpenAIError(w, http.StatusBadGateway, "模型后端请求失败", "server_error", "")
		return
	}
	defer resp.Body.Close()
	defer h.metrics.RequestsTotal.WithLabelValues(backend.cfg.Name, status).Inc()

	copyResponseHeaders(w.Header(), resp.Header)
	if req.Stream && resp.Header.Get("Content-Type") == "" && resp.StatusCode < 400 {
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	}
	w.WriteHeader(resp.StatusCode)

	if req.Stream {
		streamCopy(w, resp.Body)
		return
	}
	_, _ = io.Copy(w, resp.Body)
}

func mockContent(req chatCompletionRequest, backend modelBackend) string {
	lastUser := ""
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == "user" {
			lastUser = messageContentText(req.Messages[i])
			break
		}
	}
	if strings.TrimSpace(lastUser) == "" && len(req.Messages) > 0 {
		lastUser = messageContentText(req.Messages[len(req.Messages)-1])
	}
	if strings.TrimSpace(lastUser) == "" {
		lastUser = "空消息"
	}

	return fmt.Sprintf("%s 已收到请求：%s", backend.cfg.Name, lastUser)
}

func messageContentText(message chatMessage) string {
	if len(message.Content) == 0 {
		return ""
	}

	var text string
	if err := json.Unmarshal(message.Content, &text); err == nil {
		return text
	}

	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(message.Content, &parts); err == nil {
		texts := make([]string, 0, len(parts))
		for _, part := range parts {
			if part.Type == "" || part.Type == "text" {
				texts = append(texts, part.Text)
			}
		}
		return strings.Join(texts, "")
	}

	return string(message.Content)
}

func writeSSE(w io.Writer, flusher http.Flusher, payload any) bool {
	data, err := json.Marshal(payload)
	if err != nil {
		return false
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
		return false
	}
	flusher.Flush()
	return true
}

func streamCopy(w http.ResponseWriter, body io.Reader) {
	flusher, ok := w.(http.Flusher)
	reader := bufio.NewReader(body)
	buf := make([]byte, 4096)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				return
			}
			if ok {
				flusher.Flush()
			}
		}
		if err != nil {
			return
		}
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeOpenAIError(w http.ResponseWriter, status int, message, errorType, param string) {
	var paramPtr *string
	if param != "" {
		paramPtr = &param
	}

	writeJSON(w, status, errorResponse{
		Error: errorBody{
			Message: message,
			Type:    errorType,
			Param:   paramPtr,
			Code:    nil,
		},
	})
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

func splitText(text string, chunkSize int) []string {
	if text == "" {
		return nil
	}
	if chunkSize <= 0 {
		chunkSize = 12
	}

	runes := []rune(text)
	chunks := make([]string, 0, (len(runes)+chunkSize-1)/chunkSize)
	for start := 0; start < len(runes); start += chunkSize {
		end := start + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[start:end]))
	}
	return chunks
}

func estimatePromptTokens(messages []chatMessage) int {
	total := 0
	for _, message := range messages {
		total += estimateTokens(message.Role)
		total += estimateTokens(messageContentText(message))
	}
	return total
}

func estimateTokens(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	count := utf8.RuneCountInString(text)
	return count/4 + 1
}

func newID(prefix string) string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "-" + hex.EncodeToString(buf[:])
}
