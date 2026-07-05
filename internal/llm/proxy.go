package llm

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/agent-gateway/telemetry-gateway/internal/config"
	"github.com/agent-gateway/telemetry-gateway/internal/tracing"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	failureReasonCircuitOpen    = "circuit_open"
	failureReasonHTTP429        = "http_429"
	failureReasonHTTP5xx        = "http_5xx"
	failureReasonNetwork        = "network"
	failureReasonStream         = "stream_error"
	failureReasonTimeout        = "timeout"
	failureReasonClientCanceled = "client_canceled"
)

type proxyAttemptResult struct {
	done      bool
	retryable bool
	reason    string
	timeout   bool
}

type attemptFailureSummary struct {
	any        bool
	allTimeout bool
}

func (s *attemptFailureSummary) observe(result proxyAttemptResult) {
	if !s.any {
		s.allTimeout = true
	}
	s.any = true
	if !result.timeout {
		s.allTimeout = false
	}
}

func retryableAttempt(reason string) proxyAttemptResult {
	return proxyAttemptResult{
		retryable: true,
		reason:    reason,
		timeout:   reason == failureReasonTimeout,
	}
}

func doneAttempt() proxyAttemptResult {
	return proxyAttemptResult{done: true}
}

func (h *Handler) serveChatCompletionWithFallback(w http.ResponseWriter, r *http.Request, rawBody []byte, req chatCompletionRequest) {
	ctx, span := h.tracer.Start(r.Context(), "llm.chat_completion", trace.WithAttributes(
		attribute.String("gen_ai.request.model", req.Model),
		attribute.Bool("gen_ai.request.stream", req.Stream),
	))
	defer span.End()
	r = r.WithContext(ctx)

	candidates := h.candidateModels(req.Model)
	if len(candidates) == 0 {
		span.SetStatus(codes.Error, "missing_model_candidates")
		writeOpenAIError(w, http.StatusNotFound, "未找到可用模型候选", "invalid_request_error", "model")
		return
	}

	var failures attemptFailureSummary
	//循环尝试候选的所有模型
	for index, model := range candidates {
		backend := h.backends[model]
		candidateReq := req
		candidateReq.Model = model

		candidateBody := rawBody
		//重写模型
		if model != req.Model {
			var err error
			candidateBody, err = rewriteRequestModel(rawBody, model)
			if err != nil {
				writeOpenAIError(w, http.StatusInternalServerError, "改写降级模型请求失败", "server_error", "")
				return
			}
		}

		if backend.backendType == "mock" {
			h.serveMock(w, r, candidateReq, backend)
			return
		}

		result := h.proxyOpenAICompatible(w, r, candidateBody, candidateReq, backend)
		if result.done {
			return
		}
		if !result.retryable {
			writeOpenAIError(w, http.StatusServiceUnavailable, "模型后端暂不可用", "server_error", "")
			return
		}

		failures.observe(result)
		if index+1 < len(candidates) {
			nextModel := candidates[index+1]
			h.logger.Warn(
				"模型请求触发降级",
				"trace_id", tracing.TraceIDFromContext(r.Context()),
				"from_model", model,
				"to_model", nextModel,
				"backend", backend.cfg.Name,
				"reason", result.reason,
			)
			span.AddEvent("llm.fallback", trace.WithAttributes(
				attribute.String("from_model", model),
				attribute.String("to_model", nextModel),
				attribute.String("reason", result.reason),
			))
			h.observeFallback(r.Context(), model, nextModel, result.reason)
			continue
		}
	}

	status := http.StatusServiceUnavailable
	message := "模型后端暂不可用"
	if failures.any && failures.allTimeout {
		status = http.StatusGatewayTimeout
		message = "模型后端请求超时"
	}
	span.SetStatus(codes.Error, message)
	writeOpenAIError(w, status, message, "server_error", "")
}

func (h *Handler) proxyOpenAICompatible(w http.ResponseWriter, r *http.Request, rawBody []byte, req chatCompletionRequest, backend modelBackend) proxyAttemptResult {
	ctx, span := h.tracer.Start(r.Context(), "llm.backend_request", trace.WithAttributes(
		attribute.String("llm.backend", backend.cfg.Name),
		attribute.String("gen_ai.request.model", req.Model),
		attribute.Bool("gen_ai.request.stream", req.Stream),
	))
	defer span.End()
	r = r.WithContext(ctx)

	permit, ok := h.allowBackendRequest(backend.cfg.Name)
	if !ok {
		span.SetAttributes(attribute.String("llm.failure_reason", failureReasonCircuitOpen))
		span.SetStatus(codes.Error, failureReasonCircuitOpen)
		return retryableAttempt(failureReasonCircuitOpen)
	}

	//开始计时,测量首Token及请求延迟
	start := time.Now()
	result := requestResultFailure
	defer func() {
		duration := time.Since(start)
		h.observeBackendRequest(r.Context(), backend.cfg.Name, result, duration)
		span.SetAttributes(
			attribute.String("llm.result", result),
			attribute.Int64("llm.duration_ms", duration.Milliseconds()),
		)
		if result != requestResultSuccess {
			span.SetStatus(codes.Error, result)
		}
	}()

	if req.Stream {
		return h.proxyOpenAICompatibleStream(w, r, rawBody, req, backend, &permit, start, &result)
	}

	return h.proxyOpenAICompatibleNonStream(w, r, rawBody, backend, &permit, &result)
}

func (h *Handler) proxyOpenAICompatibleNonStream(w http.ResponseWriter, r *http.Request, rawBody []byte, backend modelBackend, permit *circuitBreakerPermit, result *string) proxyAttemptResult {
	ctx, cancel := context.WithTimeout(r.Context(), h.requestTimeout)
	defer cancel()

	//构造http请求
	upstreamReq, err := h.newUpstreamRequest(ctx, r, rawBody, backend)
	if err != nil {
		permit.Ignore()
		writeOpenAIError(w, http.StatusBadGateway, "创建上游请求失败", "server_error", "")
		return doneAttempt()
	}

	//发送请求
	resp, err := h.client.Do(upstreamReq)
	if err != nil {
		reason := failureReasonFromError(err)
		if reason == failureReasonClientCanceled {
			permit.Ignore()
			return doneAttempt()
		}
		permit.Fail()
		h.observeUpstreamError(r.Context(), backend.cfg.Name, reason)
		h.logger.Warn(
			"模型后端请求失败",
			"trace_id", tracing.TraceIDFromContext(r.Context()),
			"backend", backend.cfg.Name,
			"reason", reason,
			"error", err,
		)
		trace.SpanFromContext(r.Context()).RecordError(err)
		trace.SpanFromContext(r.Context()).SetAttributes(attribute.String("llm.failure_reason", reason))
		return retryableAttempt(reason)
	}
	defer resp.Body.Close()

	//判断请求是否成功
	*result = requestResultFromStatus(resp.StatusCode)
	if isRetryableStatus(resp.StatusCode) {
		reason := failureReasonFromStatus(resp.StatusCode)
		permit.Fail()
		h.observeUpstreamError(r.Context(), backend.cfg.Name, reason)
		trace.SpanFromContext(r.Context()).SetAttributes(
			attribute.String("llm.failure_reason", reason),
			attribute.Int("http.response.status_code", resp.StatusCode),
		)
		return retryableAttempt(reason)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		reason := failureReasonFromError(err)
		if reason == failureReasonClientCanceled {
			permit.Ignore()
			return doneAttempt()
		}
		*result = requestResultFailure
		permit.Fail()
		h.observeUpstreamError(r.Context(), backend.cfg.Name, reason)
		trace.SpanFromContext(r.Context()).RecordError(err)
		trace.SpanFromContext(r.Context()).SetAttributes(attribute.String("llm.failure_reason", reason))
		return retryableAttempt(reason)
	}

	permit.Succeed()
	//复制响应头
	copyResponseHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	if _, err := w.Write(body); err != nil {
		*result = requestResultFailure
	}
	return doneAttempt()
}

func (h *Handler) newUpstreamRequest(ctx context.Context, r *http.Request, rawBody []byte, backend modelBackend) (*http.Request, error) {
	upstreamReq, err := http.NewRequestWithContext(ctx, http.MethodPost, backend.chatCompletionsURL, bytes.NewReader(rawBody))
	if err != nil {
		return nil, err
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
	tracing.Inject(upstreamReq, ctx)

	return upstreamReq, nil
}

func (h *Handler) allowBackendRequest(backendName string) (circuitBreakerPermit, bool) {
	if h.circuitBreaker == nil {
		return circuitBreakerPermit{}, true
	}
	return h.circuitBreaker.Allow(backendName)
}

func isRetryableStatus(status int) bool {
	return status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
}

func failureReasonFromStatus(status int) string {
	if status == http.StatusTooManyRequests {
		return failureReasonHTTP429
	}
	if status >= http.StatusInternalServerError {
		return failureReasonHTTP5xx
	}
	return ""
}

func failureReasonFromError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.Canceled) {
		return failureReasonClientCanceled
	}
	if errors.Is(err, context.DeadlineExceeded) || os.IsTimeout(err) {
		return failureReasonTimeout
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return failureReasonTimeout
	}
	return failureReasonNetwork
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
