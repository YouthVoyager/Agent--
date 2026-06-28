package llm

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

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
