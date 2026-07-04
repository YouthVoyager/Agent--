package tokenusage

import (
	"strings"
	"sync"
	"time"

	"github.com/agent-gateway/telemetry-gateway/internal/config"
)

const (
	defaultIdentityIdleTTL = 48 * time.Hour
)

type budgetWindow struct {
	used     int
	reserved int
}

type identityBudget struct {
	windows  map[int64]*budgetWindow
	lastSeen time.Time
}

// Reservation 表示一次请求占用的 token 预算预留。
type Reservation struct {
	limiter  *BudgetLimiter
	identity string
	windowID int64
	tokens   int
	done     bool
}

// BudgetSnapshot 表示当前身份在预算窗口内的可用状态。
type BudgetSnapshot struct {
	Limit     int
	Used      int
	Reserved  int
	Remaining int
	ResetAt   time.Time
}

// BudgetLimiter 按身份维护 token 预算窗口。
type BudgetLimiter struct {
	identityHeader             string
	window                     time.Duration
	defaultBudgetTokens        int
	defaultMaxCompletionTokens int
	userBudgets                map[string]int
	identityIdleTTL            time.Duration
	now                        func() time.Time

	mu       sync.Mutex
	accounts map[string]*identityBudget
}

// NewBudgetLimiter 创建 token 预算限制器。
func NewBudgetLimiter(cfg config.TokenUsageConfig) *BudgetLimiter {
	userBudgets := make(map[string]int, len(cfg.UserBudgets))
	for identity, budget := range cfg.UserBudgets {
		userBudgets[strings.TrimSpace(identity)] = budget
	}

	return &BudgetLimiter{
		identityHeader:             strings.TrimSpace(cfg.IdentityHeader),
		window:                     cfg.Window.Duration,
		defaultBudgetTokens:        cfg.DefaultBudgetTokens,
		defaultMaxCompletionTokens: cfg.DefaultMaxCompletionTokens,
		userBudgets:                userBudgets,
		identityIdleTTL:            defaultIdentityIdleTTL,
		now:                        time.Now,
		accounts:                   make(map[string]*identityBudget),
	}
}

func (l *BudgetLimiter) defaultCompletionBudget() int {
	if l == nil {
		return 0
	}
	return l.defaultMaxCompletionTokens
}

func (l *BudgetLimiter) reserve(identity string, tokens int) (*Reservation, BudgetSnapshot, bool) {
	if l == nil {
		return nil, BudgetSnapshot{}, true
	}
	if tokens <= 0 {
		tokens = 1
	}

	now := l.now()
	windowID := l.windowID(now)

	l.mu.Lock()
	defer l.mu.Unlock()

	l.cleanupLocked(now)

	account := l.accountLocked(identity, now)
	window := account.windows[windowID]
	limit := l.budgetForIdentity(identity)
	snapshot := l.snapshotLocked(window, limit, windowID)
	if tokens > snapshot.Remaining {
		return nil, snapshot, false
	}

	window.reserved += tokens
	snapshot = l.snapshotLocked(window, limit, windowID)
	return &Reservation{
		limiter:  l,
		identity: identity,
		windowID: windowID,
		tokens:   tokens,
	}, snapshot, true
}

func (l *BudgetLimiter) snapshot(identity string) BudgetSnapshot {
	if l == nil {
		return BudgetSnapshot{}
	}
	now := l.now()
	windowID := l.windowID(now)

	l.mu.Lock()
	defer l.mu.Unlock()

	account := l.accountLocked(identity, now)
	return l.snapshotLocked(account.windows[windowID], l.budgetForIdentity(identity), windowID)
}

// Commit 将预留预算修正为请求完成后的实际 token 用量。
func (r *Reservation) Commit(actualTokens int) BudgetSnapshot {
	if r == nil || r.limiter == nil || r.done {
		return BudgetSnapshot{}
	}
	if actualTokens < 0 {
		actualTokens = 0
	}

	r.done = true
	l := r.limiter

	l.mu.Lock()
	defer l.mu.Unlock()

	account := l.accountLocked(r.identity, l.now())
	window := l.windowLocked(account, r.windowID)
	if window.reserved >= r.tokens {
		window.reserved -= r.tokens
	} else {
		window.reserved = 0
	}
	window.used += actualTokens
	return l.snapshotLocked(window, l.budgetForIdentity(r.identity), r.windowID)
}

// Release 释放预留预算，不记录实际用量。
func (r *Reservation) Release() BudgetSnapshot {
	if r == nil || r.limiter == nil || r.done {
		return BudgetSnapshot{}
	}

	r.done = true
	l := r.limiter

	l.mu.Lock()
	defer l.mu.Unlock()

	account := l.accountLocked(r.identity, l.now())
	window := l.windowLocked(account, r.windowID)
	if window.reserved >= r.tokens {
		window.reserved -= r.tokens
	} else {
		window.reserved = 0
	}
	return l.snapshotLocked(window, l.budgetForIdentity(r.identity), r.windowID)
}

func (l *BudgetLimiter) accountLocked(identity string, now time.Time) *identityBudget {
	account, ok := l.accounts[identity]
	if !ok {
		account = &identityBudget{
			windows: make(map[int64]*budgetWindow),
		}
		l.accounts[identity] = account
	}
	account.lastSeen = now

	windowID := l.windowID(now)
	if _, ok := account.windows[windowID]; !ok {
		account.windows[windowID] = &budgetWindow{}
	}
	return account
}

func (l *BudgetLimiter) windowLocked(account *identityBudget, windowID int64) *budgetWindow {
	window, ok := account.windows[windowID]
	if !ok {
		window = &budgetWindow{}
		account.windows[windowID] = window
	}
	return window
}

func (l *BudgetLimiter) snapshotLocked(window *budgetWindow, limit int, windowID int64) BudgetSnapshot {
	used := 0
	reserved := 0
	if window != nil {
		used = window.used
		reserved = window.reserved
	}
	remaining := limit - used - reserved
	if remaining < 0 {
		remaining = 0
	}
	return BudgetSnapshot{
		Limit:     limit,
		Used:      used,
		Reserved:  reserved,
		Remaining: remaining,
		ResetAt:   l.resetAt(windowID),
	}
}

func (l *BudgetLimiter) cleanupLocked(now time.Time) {
	if l.identityIdleTTL <= 0 {
		return
	}
	currentWindowID := l.windowID(now)
	for identity, account := range l.accounts {
		if now.Sub(account.lastSeen) >= l.identityIdleTTL {
			delete(l.accounts, identity)
			continue
		}
		for windowID, window := range account.windows {
			if windowID < currentWindowID-1 && window.reserved == 0 {
				delete(account.windows, windowID)
			}
		}
	}
}

func (l *BudgetLimiter) budgetForIdentity(identity string) int {
	if budget, ok := l.userBudgets[identity]; ok {
		return budget
	}
	return l.defaultBudgetTokens
}

func (l *BudgetLimiter) windowID(now time.Time) int64 {
	return now.UnixNano() / l.window.Nanoseconds()
}

func (l *BudgetLimiter) resetAt(windowID int64) time.Time {
	return time.Unix(0, (windowID+1)*l.window.Nanoseconds())
}
