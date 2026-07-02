package config

type Config struct {
	Server        ServerConfig        `json:"server"`
	Observability ObservabilityConfig `json:"observability"`
	Auth          AuthConfig          `json:"auth"`
	RateLimit     RateLimitConfig     `json:"rate_limit"`
	AI            AIConfig            `json:"ai"`
}

type ServerConfig struct {
	Address           string   `json:"address"`
	ReadHeaderTimeout Duration `json:"read_header_timeout"`
	ShutdownTimeout   Duration `json:"shutdown_timeout"`
}

type ObservabilityConfig struct {
	MetricsNamespace string `json:"metrics_namespace"`
}

type RateLimitConfig struct {
	User UserRateLimitConfig `json:"user"`
}

type UserRateLimitConfig struct {
	Enabled           bool    `json:"enabled"`
	IdentityHeader    string  `json:"identity_header"`
	RequestsPerSecond float64 `json:"requests_per_second"`
	Burst             int     `json:"burst"`
}

type AIConfig struct {
	RequestTimeout Duration             `json:"request_timeout"`
	Backends       []ModelBackendConfig `json:"backends"`
}

type ModelBackendConfig struct {
	Name      string   `json:"name"`
	Type      string   `json:"type"`
	BaseURL   string   `json:"base_url,omitempty"`
	APIKey    string   `json:"api_key,omitempty"`
	APIKeyEnv string   `json:"api_key_env,omitempty"`
	Models    []string `json:"models"`
}
