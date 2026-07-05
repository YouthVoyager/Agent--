package llm

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

func (h *Handler) serveMock(w http.ResponseWriter, r *http.Request, req chatCompletionRequest, backend modelBackend) {
	ctx, span := h.tracer.Start(r.Context(), "llm.backend_request", trace.WithAttributes(
		attribute.String("llm.backend", backend.cfg.Name),
		attribute.String("gen_ai.request.model", req.Model),
		attribute.Bool("gen_ai.request.stream", req.Stream),
		attribute.Bool("llm.mock", true),
	))
	defer span.End()
	r = r.WithContext(ctx)

	start := time.Now()
	result := requestResultSuccess
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
		if ok := h.streamMock(w, r, req, backend); !ok {
			result = requestResultFailure
		}
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

func (h *Handler) streamMock(w http.ResponseWriter, r *http.Request, req chatCompletionRequest, backend modelBackend) bool {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeOpenAIError(w, http.StatusInternalServerError, "当前 HTTP writer 不支持流式刷新", "server_error", "")
		return false
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
		return false
	}

	for _, piece := range splitText(content, 12) {
		select {
		case <-r.Context().Done():
			return false
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
			return false
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
		return false
	}

	if _, err := io.WriteString(w, "data: [DONE]\n\n"); err != nil {
		return false
	}
	flusher.Flush()
	return true
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
