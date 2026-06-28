package config

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

func (c Config) Validate() error {
	if strings.TrimSpace(c.Server.Address) == "" {
		return fmt.Errorf("server.address 不能为空")
	}

	_, port, err := net.SplitHostPort(c.Server.Address)
	if err != nil {
		return fmt.Errorf("server.address 必须是 host:port 或 :port 格式: %w", err)
	}

	portNum, err := strconv.Atoi(port)
	if err != nil || portNum <= 0 || portNum > 65535 {
		return fmt.Errorf("server.address 端口无效: %q", port)
	}

	if c.Server.ReadHeaderTimeout.Duration <= 0 {
		return fmt.Errorf("server.read_header_timeout 必须大于 0")
	}
	if c.Server.ShutdownTimeout.Duration <= 0 {
		return fmt.Errorf("server.shutdown_timeout 必须大于 0")
	}
	if strings.TrimSpace(c.Observability.MetricsNamespace) == "" {
		return fmt.Errorf("observability.metrics_namespace 不能为空")
	}

	return nil
}
