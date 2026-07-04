package config

type Config struct {
	Server        ServerConfig        `json:"server"`
	Observability ObservabilityConfig `json:"observability"`
	Auth          AuthConfig          `json:"auth"`
	RateLimit     RateLimitConfig     `json:"rate_limit"`
	TokenUsage    TokenUsageConfig    `json:"token_usage"`
	AI            AIConfig            `json:"ai"`
}

type ServerConfig struct {
	Address           string   `json:"address"`
	ReadHeaderTimeout Duration `json:"read_header_timeout"`
	ShutdownTimeout   Duration `json:"shutdown_timeout"`
}

type ObservabilityConfig struct {
	MetricsNamespace string        `json:"metrics_namespace"`
	Tracing          TracingConfig `json:"tracing"`
}

type TracingConfig struct {
	Enabled bool `json:"enabled"`
}

type RateLimitConfig struct {
	User        UserRateLimitConfig    `json:"user"`
	Concurrency ConcurrencyLimitConfig `json:"concurrency"`
}

type UserRateLimitConfig struct {
	Enabled           bool    `json:"enabled"`
	IdentityHeader    string  `json:"identity_header"`
	RequestsPerSecond float64 `json:"requests_per_second"`
	Burst             int     `json:"burst"`
}

type ConcurrencyLimitConfig struct {
	Enabled     bool `json:"enabled"`
	MaxInFlight int  `json:"max_in_flight"`
}

type TokenUsageConfig struct {
	Enabled                    bool           `json:"enabled"`
	IdentityHeader             string         `json:"identity_header"`
	Window                     Duration       `json:"window"`
	DefaultBudgetTokens        int            `json:"default_budget_tokens"`
	DefaultMaxCompletionTokens int            `json:"default_max_completion_tokens"`
	UserBudgets                map[string]int `json:"user_budgets"`
}

type AIConfig struct {
	RequestTimeout    Duration             `json:"request_timeout"`
	FirstTokenTimeout Duration             `json:"first_token_timeout"`
	CircuitBreaker    CircuitBreakerConfig `json:"circuit_breaker"`
	Fallbacks         map[string][]string  `json:"fallbacks"`
	Backends          []ModelBackendConfig `json:"backends"`
}

type CircuitBreakerConfig struct {
	Enabled             bool     `json:"enabled"`
	FailureThreshold    int      `json:"failure_threshold"`
	OpenTimeout         Duration `json:"open_timeout"`
	HalfOpenMaxRequests int      `json:"half_open_max_requests"`
}

type ModelBackendConfig struct {
	Name      string   `json:"name"`
	Type      string   `json:"type"`
	BaseURL   string   `json:"base_url,omitempty"`
	APIKey    string   `json:"api_key,omitempty"`
	APIKeyEnv string   `json:"api_key_env,omitempty"`
	Models    []string `json:"models"`
}
