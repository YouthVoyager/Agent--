package tokenusage

import (
	"encoding/json"
	"strings"
	"unicode/utf8"
)

type requestPayload struct {
	Model               string        `json:"model"`
	Messages            []chatMessage `json:"messages"`
	MaxTokens           *int          `json:"max_tokens"`
	MaxCompletionTokens *int          `json:"max_completion_tokens"`
}

type chatMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// Usage 表示一次聊天补全请求实际或估算的 token 用量。
type Usage struct {
	Model            string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	Estimated        bool
}

// RequestEstimate 表示进入模型前可以从请求体得到的 token 估算。
type RequestEstimate struct {
	Usage
	CompletionBudget int
	Reservable       bool
}

func estimateRequest(rawBody []byte, defaultMaxCompletionTokens int) (RequestEstimate, error) {
	var payload requestPayload
	if err := json.Unmarshal(rawBody, &payload); err != nil {
		return RequestEstimate{}, err
	}

	model := strings.TrimSpace(payload.Model)
	if model == "" || len(payload.Messages) == 0 {
		return RequestEstimate{
			Usage: Usage{
				Model:     model,
				Estimated: true,
			},
		}, nil
	}

	promptTokens := estimatePromptTokens(payload.Messages)
	completionBudget := completionBudgetFromRequest(payload, defaultMaxCompletionTokens)
	totalTokens := promptTokens + completionBudget
	if totalTokens <= 0 {
		totalTokens = 1
	}

	return RequestEstimate{
		Usage: Usage{
			Model:            model,
			PromptTokens:     promptTokens,
			CompletionTokens: completionBudget,
			TotalTokens:      totalTokens,
			Estimated:        true,
		},
		CompletionBudget: completionBudget,
		Reservable:       true,
	}, nil
}

func completionBudgetFromRequest(payload requestPayload, defaultMaxCompletionTokens int) int {
	if payload.MaxCompletionTokens != nil && *payload.MaxCompletionTokens > 0 {
		return *payload.MaxCompletionTokens
	}
	if payload.MaxTokens != nil && *payload.MaxTokens > 0 {
		return *payload.MaxTokens
	}
	if defaultMaxCompletionTokens > 0 {
		return defaultMaxCompletionTokens
	}
	return 0
}

func estimatePromptTokens(messages []chatMessage) int {
	total := 0
	for _, message := range messages {
		total += estimateTokens(message.Role)
		total += estimateTokens(messageContentText(message))
	}
	return total
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

func estimateTokens(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	count := utf8.RuneCountInString(text)
	return count/4 + 1
}
