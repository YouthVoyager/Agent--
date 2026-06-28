package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type Config struct {
	Server        ServerConfig        `json:"server"`
	Observability ObservabilityConfig `json:"observability"`
}

type ServerConfig struct {
	Address           string   `json:"address"`
	ReadHeaderTimeout Duration `json:"read_header_timeout"`
	ShutdownTimeout   Duration `json:"shutdown_timeout"`
}

type ObservabilityConfig struct {
	MetricsNamespace string `json:"metrics_namespace"`
}

// Duration 支持在 JSON 配置中使用 "5s" 这类可读写法。
type Duration struct {
	time.Duration
}

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
	}
}

func (d *Duration) UnmarshalJSON(data []byte) error {
	var raw string
	if err := json.Unmarshal(data, &raw); err == nil {
		value, err := time.ParseDuration(raw)
		if err != nil {
			return fmt.Errorf("解析 duration %q: %w", raw, err)
		}
		d.Duration = value
		return nil
	}

	var nanos int64
	if err := json.Unmarshal(data, &nanos); err == nil {
		d.Duration = time.Duration(nanos)
		return nil
	}

	return errors.New("duration 必须是字符串或纳秒整数")
}
