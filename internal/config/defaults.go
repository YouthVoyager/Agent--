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
