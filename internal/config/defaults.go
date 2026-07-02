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
			RequestTimeout: Duration{Duration: 30 * time.Second},
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
