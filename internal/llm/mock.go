package llm

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

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
