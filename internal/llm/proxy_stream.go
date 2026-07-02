package llm

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/http"
	"time"
)

type sseReadResult struct {
	event sseEvent
	err   error
}

func (h *Handler) proxyOpenAICompatibleStream(
	w http.ResponseWriter,
	r *http.Request,
	rawBody []byte,
	req chatCompletionRequest,
	backend modelBackend,
	permit *circuitBreakerPermit,
	start time.Time,
	result *string,
) proxyAttemptResult {
	//不支持流式输出
	if _, ok := w.(http.Flusher); !ok {
		permit.Ignore()
		writeOpenAIError(w, http.StatusInternalServerError, "当前 HTTP writer 不支持流式刷新", "server_error", "")
		return doneAttempt()
	}
	//构造请求头
	upstreamReq, err := h.newUpstreamRequest(r.Context(), r, rawBody, backend)
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
		h.observeUpstreamError(backend.cfg.Name, reason)
		h.logger.Warn("模型后端流式请求失败", "backend", backend.cfg.Name, "reason", reason, "error", err)
		return retryableAttempt(reason)
	}
	defer resp.Body.Close()
	//检验请求是否成功
	*result = requestResultFromStatus(resp.StatusCode)
	if isRetryableStatus(resp.StatusCode) {
		reason := failureReasonFromStatus(resp.StatusCode)
		permit.Fail()
		h.observeUpstreamError(backend.cfg.Name, reason)
		return retryableAttempt(reason)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		permit.Succeed()
		copyResponseHeaders(w.Header(), resp.Header)
		w.WriteHeader(resp.StatusCode)
		if _, err := io.Copy(w, resp.Body); err != nil {
			*result = requestResultFailure
		}
		return doneAttempt()
	}

	reader := bufio.NewReader(resp.Body)
	//读取首Token
	buffered, firstToken, readErr := h.readUntilFirstContentToken(r.Context(), reader, resp.Body)
	if readErr != nil {
		reason := failureReasonFromError(readErr)
		if reason == failureReasonClientCanceled {
			permit.Ignore()
			return doneAttempt()
		}
		*result = requestResultFailure
		permit.Fail()
		h.observeUpstreamError(backend.cfg.Name, reason)
		return retryableAttempt(reason)
	}

	copyResponseHeaders(w.Header(), resp.Header)
	if resp.Header.Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	}
	w.WriteHeader(resp.StatusCode)

	flusher := w.(http.Flusher)
	//先发送首Token,并校验是否能够发送成功
	if !writeBufferedSSE(w, flusher, buffered) {
		*result = requestResultFailure
		permit.Ignore()
		return doneAttempt()
	}
	if firstToken && h.metrics != nil && h.metrics.FirstTokenDuration != nil {
		h.metrics.FirstTokenDuration.WithLabelValues(backend.cfg.Name).Observe(time.Since(start).Seconds())
	}
	//开始发送
	if ok := streamCopyFromReader(w, reader, h.metrics, backend.cfg.Name, start, firstToken); !ok {
		*result = requestResultFailure
		permit.Fail()
		h.observeUpstreamError(backend.cfg.Name, failureReasonStream)
		return doneAttempt()
	}
	permit.Succeed()
	return doneAttempt()
}
//读取流直到读取到第一个token
func (h *Handler) readUntilFirstContentToken(ctx context.Context, reader *bufio.Reader, body io.Closer) ([]sseEvent, bool, error) {
	buffered := make([]sseEvent, 0, 4)
	for {
		event, err := readSSEEventWithTimeout(ctx, reader, body, h.firstTokenTimeout)
		if event.Data != "" {
			buffered = append(buffered, event)
			if hasContentToken(event.Data) {
				return buffered, true, nil
			}
		}
		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) {
			return buffered, false, nil
		}
		return buffered, false, err
	}
}

func readSSEEventWithTimeout(ctx context.Context, reader *bufio.Reader, body io.Closer, timeout time.Duration) (sseEvent, error) {
	resultCh := make(chan sseReadResult, 1)
	go func() {
		event, err := readSSEEvent(reader)
		resultCh <- sseReadResult{event: event, err: err}
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case result := <-resultCh:
		return result.event, result.err
	case <-ctx.Done():
		_ = body.Close()
		return sseEvent{}, ctx.Err()
	case <-timer.C:
		_ = body.Close()
		return sseEvent{}, context.DeadlineExceeded
	}
}
