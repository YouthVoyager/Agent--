package llm

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

func newID(prefix string) string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "-" + hex.EncodeToString(buf[:])
}
