package tracing

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync/atomic"
	"time"
)

var fallbackCounter uint64

func newTraceID() string {
	return newNonZeroHex(16)
}

func newSpanID() string {
	return newNonZeroHex(8)
}

func newNonZeroHex(byteLen int) string {
	buf := make([]byte, byteLen)
	if _, err := rand.Read(buf); err != nil {
		return fallbackRandomHex(byteLen)
	}
	if allZeroBytes(buf) {
		return newNonZeroHex(byteLen)
	}
	return hex.EncodeToString(buf)
}

func fallbackRandomHex(byteLen int) string {
	seed := []byte(time.Now().Format(time.RFC3339Nano))
	count := atomic.AddUint64(&fallbackCounter, 1)
	seed = append(seed, byte(count), byte(count>>8), byte(count>>16), byte(count>>24))
	sum := sha256.Sum256(seed)
	return hex.EncodeToString(sum[:byteLen])
}

func normalizeTraceID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) != 32 || !isHex(value) || allZeroString(value) {
		return ""
	}
	return value
}

func normalizeSpanID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) != 16 || !isHex(value) || allZeroString(value) {
		return ""
	}
	return value
}

func isValidTraceFlags(value string) bool {
	return len(value) == 2 && isHex(value)
}

func isHex(value string) bool {
	for _, char := range value {
		if (char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') {
			continue
		}
		return false
	}
	return true
}

func allZeroString(value string) bool {
	for _, char := range value {
		if char != '0' {
			return false
		}
	}
	return true
}

func allZeroBytes(value []byte) bool {
	for _, item := range value {
		if item != 0 {
			return false
		}
	}
	return true
}
