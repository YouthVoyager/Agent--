package tokenusage

import (
	"bytes"
	"net/http"
)

type captureWriter struct {
	http.ResponseWriter
	status    int
	body      bytes.Buffer
	limit     int
	written   bool
	truncated bool
}

type captureFlusher struct {
	*captureWriter
}

func newCaptureWriter(w http.ResponseWriter, limit int) (http.ResponseWriter, *captureWriter) {
	capture := &captureWriter{
		ResponseWriter: w,
		limit:          limit,
	}
	if _, ok := w.(http.Flusher); ok {
		return &captureFlusher{captureWriter: capture}, capture
	}
	return capture, capture
}

func (w *captureWriter) WriteHeader(status int) {
	if w.written {
		return
	}
	w.status = status
	w.written = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *captureWriter) Write(p []byte) (int, error) {
	if !w.written {
		w.WriteHeader(http.StatusOK)
	}
	w.capture(p)
	return w.ResponseWriter.Write(p)
}

func (w *captureWriter) Status() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

func (w *captureWriter) Body() []byte {
	return w.body.Bytes()
}

func (w *captureWriter) Truncated() bool {
	return w.truncated
}

func (w *captureWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *captureWriter) capture(p []byte) {
	if w.limit <= 0 || w.truncated {
		w.truncated = true
		return
	}

	remaining := w.limit - w.body.Len()
	if remaining <= 0 {
		w.truncated = true
		return
	}
	if len(p) > remaining {
		w.body.Write(p[:remaining])
		w.truncated = true
		return
	}
	w.body.Write(p)
}

func (w *captureFlusher) Flush() {
	if !w.written {
		w.WriteHeader(http.StatusOK)
	}
	flusher, ok := w.ResponseWriter.(http.Flusher)
	if ok {
		flusher.Flush()
	}
}
