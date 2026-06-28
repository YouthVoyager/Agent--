package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
)

func Load(path string) (Config, error) {
	cfg := Default()

	if path == "" {
		path = os.Getenv("GATEWAY_CONFIG")
	}

	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return Config{}, fmt.Errorf("读取配置文件: %w", err)
		}

		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&cfg); err != nil {
			return Config{}, fmt.Errorf("解析配置文件: %w", err)
		}
	}

	applyEnvOverrides(&cfg)

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func applyEnvOverrides(cfg *Config) {
	if value := os.Getenv("GATEWAY_ADDRESS"); value != "" {
		cfg.Server.Address = value
	}
	if value := os.Getenv("GATEWAY_ADDR"); value != "" {
		cfg.Server.Address = value
	}
}
