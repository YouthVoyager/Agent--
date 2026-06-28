package llm

import (
	"encoding/json"
	"strings"
	"unicode/utf8"
)

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
