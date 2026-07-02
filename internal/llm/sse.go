package llm

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/agent-gateway/telemetry-gateway/internal/observability"
)

type sseEvent struct {
	Data string
}

func readSSEEvent(reader *bufio.Reader) (sseEvent, error) {
	var dataLines []string

	for {
		line, err := reader.ReadString('\n')
		if err != nil && len(line) == 0 {
			return sseEvent{}, err
		}

		line = strings.TrimRight(line, "\r\n")

		// 空行表示一个 event 结束
		if line == "" {
			return sseEvent{
				Data: strings.Join(dataLines, "\n"),
			}, nil
		}

		// 注释或心跳
		if strings.HasPrefix(line, ":") {
			continue
		}

		field, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		if strings.HasPrefix(value, " ") {
			value = value[1:]
		}

		if field == "data" {
			dataLines = append(dataLines, value)
		}

		if err != nil {
			return sseEvent{
				Data: strings.Join(dataLines, "\n"),
			}, err
		}
	}
}

type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}

func hasContentToken(data string) bool {
	if data == "" || data == "[DONE]" {
		return false
	}

	var chunk streamChunk
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return false
	}

	for _, choice := range chunk.Choices {
		if choice.Delta.Content != "" {
			return true
		}
	}
	return false
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

func streamCopy(w http.ResponseWriter, body io.Reader, metrics *observability.Metrics, model string, start time.Time) bool {
	//标记首token
	firstContentTokenObserved := false
	flusher, ok := w.(http.Flusher)
	if !ok {
		return false
	}
	reader := bufio.NewReader(body)
	for {
		event, err := readSSEEvent(reader)
		//判断是否为数据包
		if event.Data != "" {
			// 先把原始 SSE event 透传给客户端
			_, writeErr := fmt.Fprintf(w, "data: %s\n\n", event.Data)
			if writeErr != nil {
				return false
			}
			flusher.Flush()

			// 成功写出并 flush 后，再统计首 content token
			if !firstContentTokenObserved && hasContentToken(event.Data) {
				firstContentTokenObserved = true
				if metrics != nil && metrics.FirstTokenDuration != nil {
					metrics.FirstTokenDuration.WithLabelValues(model).Observe(time.Since(start).Seconds())
				}
			}
		}
		if err != nil {
			return errors.Is(err, io.EOF)
		}
	}
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
