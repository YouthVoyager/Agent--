package tokenusage

import (
	"bytes"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/agent-gateway/telemetry-gateway/internal/auth"
	"github.com/agent-gateway/telemetry-gateway/internal/config"
	"github.com/agent-gateway/telemetry-gateway/internal/observability"
)

const (
	maxRequestBodyBytes  = 10 << 20
	maxResponseBodyBytes = 1 << 20
)

// Controller 将 token 用量统计和预算控制接入聊天补全请求。
type Controller struct {
	limiter *BudgetLimiter
	metrics *observability.Metrics
}

// NewController 创建 token 用量控制器。
func NewController(cfg config.TokenUsageConfig, metrics *observability.Metrics) *Controller {
	return &Controller{
		limiter: NewBudgetLimiter(cfg),
		metrics: metrics,
	}
}

// Middleware 返回 token 预算 HTTP 中间件。
func (c *Controller) Middleware(next http.Handler) http.Handler {
	if c == nil || c.limiter == nil {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			next.ServeHTTP(w, r)
			return
		}

		rawBody, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBodyBytes))
		if err != nil {
			writeTokenUsageError(w, http.StatusRequestEntityTooLarge, "请求体过大或读取失败", "invalid_request_error")
			return
		}
		restoreRequestBody(r, rawBody)

		estimate, err := estimateRequest(rawBody, c.limiter.defaultCompletionBudget())
		if err != nil || !estimate.Reservable {
			next.ServeHTTP(w, r)
			return
		}

		identity := identityFromRequest(r, c.limiter.identityHeader)
		if identity == "" {
			writeTokenUsageError(
				w,
				http.StatusUnauthorized,
				"缺少用户标识请求头 "+c.limiter.identityHeader,
				"invalid_request_error",
			)
			return
		}

		reservation, snapshot, ok := c.limiter.reserve(identity, estimate.TotalTokens)
		if !ok {
			c.observeBudgetRejected(identity, estimate.Model)
			writeBudgetHeaders(w.Header(), snapshot)
			w.Header().Set("Retry-After", retryAfter(snapshot.ResetAt, time.Now()))
			writeTokenUsageError(w, http.StatusTooManyRequests, "token 预算不足，请在预算窗口重置后重试", "rate_limit_error")
			return
		}

		writeBudgetHeaders(w.Header(), snapshot)

		responseWriter, capture := newCaptureWriter(w, maxResponseBodyBytes)
		released := false
		defer func() {
			if !released {
				snapshot := reservation.Release()
				c.observeBudgetRemaining(identity, snapshot)
			}
		}()

		next.ServeHTTP(responseWriter, r)

		status := capture.Status()
		if status < http.StatusOK || status >= http.StatusMultipleChoices {
			snapshot := reservation.Release()
			c.observeBudgetRemaining(identity, snapshot)
			released = true
			return
		}

		usage := usageFromResponseBody(capture.Body(), capture.Header().Get("Content-Type"), capture.Truncated(), estimate)
		snapshot = reservation.Commit(usage.TotalTokens)
		c.observeUsage(identity, usage)
		c.observeBudgetRemaining(identity, snapshot)
		released = true
	})
}

func restoreRequestBody(r *http.Request, rawBody []byte) {
	r.Body = io.NopCloser(bytes.NewReader(rawBody))
	r.ContentLength = int64(len(rawBody))
}

func identityFromRequest(r *http.Request, fallbackHeader string) string {
	if identity, ok := auth.IdentityFromContext(r.Context()); ok {
		if userID := strings.TrimSpace(identity.UserID); userID != "" {
			return userID
		}
		if tenantID := strings.TrimSpace(identity.TenantID); tenantID != "" {
			return tenantID
		}
	}
	return strings.TrimSpace(r.Header.Get(fallbackHeader))
}

func writeBudgetHeaders(header http.Header, snapshot BudgetSnapshot) {
	header.Set("X-Token-Budget-Limit", strconv.Itoa(snapshot.Limit))
	header.Set("X-Token-Budget-Remaining", strconv.Itoa(snapshot.Remaining))
	header.Set("X-Token-Budget-Reset", strconv.FormatInt(snapshot.ResetAt.Unix(), 10))
}

func retryAfter(resetAt time.Time, now time.Time) string {
	seconds := int(math.Ceil(resetAt.Sub(now).Seconds()))
	if seconds < 1 {
		seconds = 1
	}
	return strconv.Itoa(seconds)
}

func (c *Controller) observeUsage(identity string, usage Usage) {
	if c == nil || c.metrics == nil {
		return
	}
	c.metrics.ObserveTokenUsage(
		identity,
		usage.Model,
		usage.PromptTokens,
		usage.CompletionTokens,
		usage.TotalTokens,
		usage.Estimated,
	)
}

func (c *Controller) observeBudgetRemaining(identity string, snapshot BudgetSnapshot) {
	if c == nil || c.metrics == nil {
		return
	}
	c.metrics.SetTokenBudgetRemaining(identity, snapshot.Remaining)
}

func (c *Controller) observeBudgetRejected(identity, model string) {
	if c == nil || c.metrics == nil {
		return
	}
	c.metrics.ObserveTokenBudgetRejected(identity, model)
}
