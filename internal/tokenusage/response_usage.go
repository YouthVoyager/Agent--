package tokenusage

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
)

type responsePayload struct {
	Model   string           `json:"model"`
	Choices []responseChoice `json:"choices"`
	Usage   *responseUsage   `json:"usage"`
}

type responseChoice struct {
	Message responseMessage `json:"message"`
}

type responseMessage struct {
	Content string `json:"content"`
}

type responseUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type streamChunk struct {
	Model   string         `json:"model"`
	Choices []streamChoice `json:"choices"`
	Usage   *responseUsage `json:"usage"`
}

type streamChoice struct {
	Delta streamDelta `json:"delta"`
}

type streamDelta struct {
	Content string `json:"content"`
}

func usageFromResponseBody(body []byte, contentType string, truncated bool, estimate RequestEstimate) Usage {
	if truncated {
		return estimate.Usage
	}
	if strings.Contains(strings.ToLower(contentType), "text/event-stream") {
		return usageFromStreamResponse(body, estimate)
	}
	return usageFromJSONResponse(body, estimate)
}

func usageFromJSONResponse(body []byte, estimate RequestEstimate) Usage {
	var payload responsePayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return estimate.Usage
	}

	model := firstNonEmpty(payload.Model, estimate.Model)
	if payload.Usage != nil {
		return normalizeUsage(Usage{
			Model:            model,
			PromptTokens:     payload.Usage.PromptTokens,
			CompletionTokens: payload.Usage.CompletionTokens,
			TotalTokens:      payload.Usage.TotalTokens,
			Estimated:        false,
		}, estimate)
	}

	completionTokens := 0
	for _, choice := range payload.Choices {
		completionTokens += estimateTokens(choice.Message.Content)
	}
	return normalizeUsage(Usage{
		Model:            model,
		PromptTokens:     estimate.PromptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      estimate.PromptTokens + completionTokens,
		Estimated:        true,
	}, estimate)
}

func usageFromStreamResponse(body []byte, estimate RequestEstimate) Usage {
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 0, 64*1024), len(body)+1024)

	model := estimate.Model
	completionTokens := 0
	var actualUsage *responseUsage
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}

		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}

		var chunk streamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		model = firstNonEmpty(chunk.Model, model)
		if chunk.Usage != nil {
			actualUsage = chunk.Usage
			continue
		}
		for _, choice := range chunk.Choices {
			completionTokens += estimateTokens(choice.Delta.Content)
		}
	}

	if actualUsage != nil {
		return normalizeUsage(Usage{
			Model:            model,
			PromptTokens:     actualUsage.PromptTokens,
			CompletionTokens: actualUsage.CompletionTokens,
			TotalTokens:      actualUsage.TotalTokens,
			Estimated:        false,
		}, estimate)
	}

	return normalizeUsage(Usage{
		Model:            model,
		PromptTokens:     estimate.PromptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      estimate.PromptTokens + completionTokens,
		Estimated:        true,
	}, estimate)
}

func normalizeUsage(usage Usage, estimate RequestEstimate) Usage {
	usage.Model = firstNonEmpty(usage.Model, estimate.Model)
	if usage.PromptTokens < 0 {
		usage.PromptTokens = 0
	}
	if usage.CompletionTokens < 0 {
		usage.CompletionTokens = 0
	}
	if usage.TotalTokens <= 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	if usage.TotalTokens <= 0 {
		usage.TotalTokens = estimate.TotalTokens
		usage.Estimated = true
	}
	return usage
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
