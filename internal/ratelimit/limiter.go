package ratelimit

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/agent-gateway/telemetry-gateway/internal/config"
	"golang.org/x/time/rate"
)

const (
	defaultUserIdleTTL         = 10 * time.Minute
	defaultUserCleanupInterval = time.Minute
)

type userBucket struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// UserLimiter 按用户标识执行请求限流。
type UserLimiter struct {
	identityHeader  string
	requestLimit    rate.Limit
	burst           int
	userIdleTTL     time.Duration
	cleanupInterval time.Duration
	now             func() time.Time

	mu          sync.Mutex
	users       map[string]*userBucket
	lastCleanup time.Time
}

// NewUserLimiter 创建用户级限流器。
func NewUserLimiter(cfg config.UserRateLimitConfig) *UserLimiter {
	return &UserLimiter{
		identityHeader:  http.CanonicalHeaderKey(strings.TrimSpace(cfg.IdentityHeader)),
		requestLimit:    rate.Limit(cfg.RequestsPerSecond),
		burst:           cfg.Burst,
		userIdleTTL:     defaultUserIdleTTL,
		cleanupInterval: defaultUserCleanupInterval,
		now:             time.Now,
		users:           make(map[string]*userBucket),
	}
}

func (l *UserLimiter) bucket(identity string) *rate.Limiter {
	now := l.now()

	l.mu.Lock()
	defer l.mu.Unlock()

	l.cleanupLocked(now)

	bucket, ok := l.users[identity]
	if !ok {
		bucket = &userBucket{
			limiter: rate.NewLimiter(l.requestLimit, l.burst),
		}
		l.users[identity] = bucket
	}
	bucket.lastSeen = now

	return bucket.limiter
}

func (l *UserLimiter) cleanupLocked(now time.Time) {
	if !l.lastCleanup.IsZero() && now.Sub(l.lastCleanup) < l.cleanupInterval {
		return
	}
	l.lastCleanup = now

	for identity, bucket := range l.users {
		if now.Sub(bucket.lastSeen) >= l.userIdleTTL {
			delete(l.users, identity)
		}
	}
}
