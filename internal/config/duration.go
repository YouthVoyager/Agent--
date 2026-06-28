package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Duration 支持在 JSON 配置中使用 "5s" 这类可读写法。
type Duration struct {
	time.Duration
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
