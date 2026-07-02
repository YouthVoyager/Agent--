# Policy-first Telemetry Gateway

使用 Go 实现的多租户遥测数据网关。项目目标是在 OTLP、Prometheus Remote Write、StatsD 与不同观测后端之间提供认证、租户识别、策略处理、路由、限流、缓冲和回放。

```text
业务团队
  |  OTLP traces / metrics / logs
  |  Prometheus Remote Write
  |  StatsD
  v
+-----------------------------+
| Policy-first Gateway        |
|                             |
| 认证 -> 租户识别 -> 策略    |
|      -> 路由 -> 队列/WAL    |
|      -> 多后端导出          |
+-----------------------------+
  |          |          |
  v          v          v
Tempo      Mimir      Datadog/其他
```

当前初始化已包含：

- `cmd/gateway` 可运行入口；
- JSON 配置读取和环境变量覆盖；
- OpenAI 风格 `/v1/models` 与 `/v1/chat/completions` 接口；
- 两个默认 mock 模型后端，可替换为 OpenAI 兼容真实服务；
- `text/event-stream` SSE 流式输出；
- `context.Context` 生命周期和 SIGTERM 优雅退出；
- `/healthz`、`/readyz`、`/metrics`；
- `/debug/pprof`；
- `Makefile`、`Dockerfile`、CI；
- 阶段 A 的项目边界和可靠性文档。

## 快速开始

```bash
make test
make build
./bin/gateway -config configs/gateway.example.json
```

另开一个终端检查：

```bash
curl localhost:8080/healthz
curl localhost:8080/readyz
curl localhost:8080/metrics
```

默认命名空间下，LLM 请求会暴露这些 Prometheus 指标：

- `gateway_requests_total{backend,result}`：按后端和结果累计请求数；
- `gateway_request_duration_seconds{backend,result}`：完整请求总延迟；
- `gateway_request_success_rate{backend}`：进程启动以来的累计请求成功率，取值范围为 `0` 到 `1`。

检查模型列表：

```bash
curl localhost:8080/v1/models
```

非流式聊天补全：

```bash
curl localhost:8080/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "mock-a",
    "messages": [
      {"role": "user", "content": "你好"}
    ]
  }'
```

SSE 流式聊天补全：

```bash
curl -N localhost:8080/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "mock-b",
    "stream": true,
    "messages": [
      {"role": "user", "content": "请流式输出"}
    ]
  }'
```

也可以直接运行：

```bash
make run
```

## 配置

默认配置等价于：

```json
{
  "server": {
    "address": ":8080",
    "read_header_timeout": "5s",
    "shutdown_timeout": "10s"
  },
  "observability": {
    "metrics_namespace": "gateway"
  },
  "ai": {
    "request_timeout": "30s",
    "backends": [
      {
        "name": "mock-a",
        "type": "mock",
        "models": ["mock-a", "gpt-4o-mini"]
      },
      {
        "name": "mock-b",
        "type": "mock",
        "models": ["mock-b", "gpt-4.1-mini"]
      }
    ]
  }
}
```

真实模型服务可使用 OpenAI 兼容协议接入，`base_url` 可以是服务根地址或 `/v1` 地址：

```json
{
  "name": "real-a",
  "type": "openai",
  "base_url": "https://api.openai.com/v1",
  "api_key_env": "OPENAI_API_KEY",
  "models": ["gpt-4.1-mini"]
}
```

支持的环境变量：

- `GATEWAY_CONFIG`：配置文件路径；
- `GATEWAY_ADDRESS` / `GATEWAY_ADDR`：覆盖监听地址。

## 项目边界

本项目不是通用可观测性平台，也不是 OpenTelemetry Collector 的组件数量竞争者。租户、策略、配置版本、审计、回滚、后端迁移、双写和可靠性语义是核心边界。

ACK 表示网关已经按当前配置接受数据。非持久队列下，进程崩溃可能造成已 ACK 但未导出的数据丢失；持久模式下，ACK 应在 WAL 写入并满足 fsync 策略后返回。后端可能收到重复批次，下游写入需要具备幂等或去重能力。
