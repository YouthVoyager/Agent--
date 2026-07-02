package config

import "time"

func Default() Config {
	return Config{
		Server: ServerConfig{
			Address:           ":8080",
			ReadHeaderTimeout: Duration{Duration: 5 * time.Second},
			ShutdownTimeout:   Duration{Duration: 10 * time.Second},
		},
		Observability: ObservabilityConfig{
			MetricsNamespace: "gateway",
			Tracing: TracingConfig{
				Enabled: true,
			},
		},
		Auth: AuthConfig{
			APIKey: APIKeyAuthConfig{
				Enabled: false,
				Header:  "Authorization",
			},
		},
		RateLimit: RateLimitConfig{
			User: UserRateLimitConfig{
				Enabled:           false,
				IdentityHeader:    "X-User-ID",
				RequestsPerSecond: 1,
				Burst:             1,
			},
			Concurrency: ConcurrencyLimitConfig{
				Enabled:     false,
				MaxInFlight: 100,
			},
		},
		AI: AIConfig{
			RequestTimeout:    Duration{Duration: 30 * time.Second},
			FirstTokenTimeout: Duration{Duration: 30 * time.Second},
			CircuitBreaker: CircuitBreakerConfig{
				Enabled:             true,
				FailureThreshold:    3,
				OpenTimeout:         Duration{Duration: 30 * time.Second},
				HalfOpenMaxRequests: 1,
			},
			Fallbacks: map[string][]string{},
			Backends: []ModelBackendConfig{
				{
					Name:   "mock-a",
					Type:   "mock",
					Models: []string{"mock-a", "gpt-4o-mini"},
				},
				{
					Name:   "mock-b",
					Type:   "mock",
					Models: []string{"mock-b", "gpt-4.1-mini"},
				},
			},
		},
	}
}
