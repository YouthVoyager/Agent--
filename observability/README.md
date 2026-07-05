# 观测栈

本目录提供一套单节点生产形态的观测栈，用于运行和验证 Agent 网关的 Trace、Log 和 Prometheus Metric 全链路。组件版本在 `docker-compose.observability.yml` 中显式固定，生产升级应通过改版本和回归验证完成，不使用浮动 `latest`。

## 数据流

```text
Agent 网关
  | OTLP/HTTP traces / logs
  v
OpenTelemetry Collector
  | traces              | logs
  v                     v
Tempo                 Loki

Agent 网关 /metrics
  v
Prometheus
  | remote_write
  v
Mimir

Grafana
  | datasource: Mimir / Prometheus / Tempo / Loki
  v
Agent Gateway Overview
```

## 启动

```bash
make observability-up
```

服务入口：

- 网关管理后台：http://localhost:8080/admin/
- Grafana：http://localhost:3000
- Prometheus：http://localhost:9090
- Mimir：http://localhost:9009
- Tempo：http://localhost:3200
- Loki：http://localhost:3100
- Collector OTLP HTTP：http://localhost:4318
- Collector OTLP gRPC：http://localhost:4317

`configs/gateway.observability.json` 已将 `observability.opentelemetry.endpoint` 指向 `http://otel-collector:4318`，并开启 `observability.stack`。管理后台会读取 `/admin/api/observability`，展示 Grafana、Prometheus、Mimir、Tempo、Loki 和 Collector 的状态，并嵌入 Grafana 的 `Agent Gateway Overview` 看板。

## 停止

```bash
make observability-down
```

如需保留数据，直接停止容器即可。`down -v` 会删除持久化卷，不应在生产环境使用。

## 生产迁移点

- Mimir、Tempo、Loki 当前使用本地文件系统卷，生产多实例应迁移到对象存储，并开启副本、租户认证、TLS 和访问控制。
- Prometheus 当前负责抓取网关 `/metrics` 并 remote_write 到 Mimir；生产可改为 Prometheus Agent、Grafana Agent Flow 或 Alloy。
- Collector 已配置 `memory_limiter`、`batch`、重试和发送队列；生产需要按吞吐量调大队列、内存限制和副本数。
- Grafana 默认启用匿名 Viewer 和 iframe 嵌入，用于让管理后台只读展示看板。生产应通过 `GRAFANA_ADMIN_USER`、`GRAFANA_ADMIN_PASSWORD` 注入管理员账号，接入 SSO 或反向代理鉴权，并限制管理后台可嵌入的 Grafana 域名。
- `observability.stack.services[].health_url` 是网关容器内部访问地址，`public_url` 是浏览器打开地址；部署到 Kubernetes 或域名环境时应分别配置。
