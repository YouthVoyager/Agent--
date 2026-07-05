# Policy-first Telemetry Gateway

使用 Go 实现的多租户遥测数据网关。项目目标是在 OTLP、Prometheus Remote Write、StatsD 与不同观测后端之间提供认证、租户识别、策略处理、路由、限流、缓冲和回放。

```text
业务团队
  |  OTLP traces / logs
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
- 可配置的用户级请求限流；
- 可配置的聊天补全全局并发限制；
- 可配置的 token 用量统计和预算控制；
- 上游超时、熔断和模型降级；
- 请求链路追踪，支持 `traceparent` 与 `X-Trace-ID` 透传；
- OpenTelemetry Trace、Log 生成、采集和 OTLP/HTTP 导出；
- 生产形态观测栈示例：OpenTelemetry Collector、Prometheus、Mimir、Tempo、Loki、Grafana；
- `context.Context` 生命周期和 SIGTERM 优雅退出；
- 内置 React 管理页面 `/admin/`，支持查看网关状态、业务指标和观测栈状态；
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

浏览器访问 `http://localhost:8080/admin/` 可打开管理页面。页面会读取健康状态、就绪状态、Prometheus 指标、模型列表和观测栈状态；开启 API Key 鉴权时，可在页面右侧填写访问凭据。

默认启用请求链路追踪和 OpenTelemetry instrumentation。网关会优先延续入站 `traceparent`，其次使用合法的 `X-Trace-ID` / `X-Request-ID`，都不存在时生成新的 trace id。每个响应都会带上 `Traceparent`、`X-Trace-ID` 和 `X-Request-ID`，代理到上游模型服务时也会透传同一 trace id 并创建新的子 span。访问日志会记录 `trace_id`、`span_id`、HTTP 方法、路径、状态码、响应字节数和耗时。

未配置 OTLP endpoint 时，OpenTelemetry 只在进程内生成 trace 和 log，不会向本地 collector 发请求。配置 `observability.opentelemetry.endpoint` 或设置 `OTEL_EXPORTER_OTLP_ENDPOINT` 后，会通过 OTLP/HTTP 导出 Trace 和 Log；也可以分别配置 `traces.endpoint`、`logs.endpoint`。

LLM 请求会暴露这些 Prometheus 指标：

- `gateway_requests_total{backend,result}`：按后端和结果累计请求数；
- `gateway_request_duration_seconds{backend,result}`：完整请求总延迟；
- `gateway_request_success_rate{backend}`：进程启动以来的累计请求成功率，取值范围为 `0` 到 `1`。
- `aegis_first_token_duration_seconds{model}`：流式响应首个内容 token 延迟；
- `gateway_fallbacks_total{from_model,to_model,reason}`：模型降级次数；
- `gateway_upstream_errors_total{backend,reason}`：可触发容错的上游错误次数；
- `gateway_circuit_breaker_state{backend}`：后端熔断状态，`0` 表示关闭、`1` 表示半开、`2` 表示打开；
- `gateway_token_usage_total{identity,model,type,estimated}`：按身份、模型和 token 类型统计用量；
- `gateway_token_budget_remaining{identity}`：当前预算窗口内的剩余 token；
- `gateway_token_budget_rejected_total{identity,model}`：因 token 预算不足被拒绝的请求数。

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
    "metrics_namespace": "gateway",
    "tracing": {
      "enabled": true
    },
    "opentelemetry": {
      "enabled": true,
      "service_name": "telemetry-gateway",
      "service_version": "dev",
      "environment": "local",
      "endpoint": "",
      "insecure": true,
	      "headers": {},
	      "export_timeout": "10s",
	      "traces": {
	        "enabled": true,
	        "endpoint": ""
	      },
	      "logs": {
	        "enabled": true,
        "endpoint": ""
      }
    },
    "stack": {
      "enabled": false,
      "services": [],
      "dashboards": []
    }
  },
  "auth": {
    "api_key": {
      "enabled": false,
      "header": "Authorization",
      "keys": []
    }
  },
  "rate_limit": {
    "user": {
      "enabled": false,
      "identity_header": "X-User-ID",
      "requests_per_second": 1,
      "burst": 1
    },
    "concurrency": {
      "enabled": false,
      "max_in_flight": 100
    }
  },
  "token_usage": {
    "enabled": false,
    "identity_header": "X-User-ID",
    "window": "24h",
    "default_budget_tokens": 100000,
    "default_max_completion_tokens": 1024,
    "user_budgets": {}
  },
  "ai": {
    "request_timeout": "30s",
    "first_token_timeout": "30s",
    "circuit_breaker": {
      "enabled": true,
      "failure_threshold": 3,
      "open_timeout": "30s",
      "half_open_max_requests": 1
    },
    "fallbacks": {},
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

如需关闭请求链路追踪：

```json
{
  "observability": {
    "tracing": {
      "enabled": false
    }
  }
}
```

如需导出 OpenTelemetry Trace、Log 到本地 collector：

```json
{
  "observability": {
    "opentelemetry": {
      "enabled": true,
      "endpoint": "http://localhost:4318",
      "insecure": true,
	      "traces": {
	        "enabled": true
	      },
	      "logs": {
        "enabled": true
      }
    }
  }
}
```

上面的 `endpoint` 是 OTLP/HTTP base URL，网关会分别导出到 `/v1/traces` 和 `/v1/logs`。如果需要信号级地址，可以使用 `traces.endpoint`、`logs.endpoint` 指定完整 URL。

如需启动完整观测栈：

```bash
make observability-up
```

该命令会启动网关、OpenTelemetry Collector、Prometheus、Mimir、Tempo、Loki 和 Grafana。网关使用 `configs/gateway.observability.json`，OTLP Trace/Log 数据发往 Collector，Prometheus 抓取 `/metrics` 并 remote_write 到 Mimir，Grafana 预置 Mimir、Prometheus、Tempo、Loki 数据源和 `Agent Gateway Overview` 看板。管理后台 `/admin/` 会通过 `/admin/api/observability` 展示各组件状态并嵌入 Grafana 看板。更多说明见 `observability/README.md`。

API Key 鉴权默认关闭。开启后，所有 `/v1/` 请求都需要携带 `Authorization: Bearer <api_key>`；`/healthz`、`/readyz`、`/metrics` 和 `/debug/pprof` 保持公开。配置中只保存 API Key 的 SHA-256 哈希：

```bash
printf 'dev-secret-key' | shasum -a 256
```

```json
{
  "auth": {
    "api_key": {
      "enabled": true,
      "header": "Authorization",
      "keys": [
        {
          "id": "dev-key",
          "key_hash": "sha256:0537dfd229ccd644e29c82f0c27a1b3b075a1589fa75a186ed40abc25bfcd248",
          "user_id": "dev-user",
          "tenant_id": "dev",
          "scopes": ["chat:completions", "models:read"]
        }
      ]
    }
  }
}
```

启用后请求示例：

```bash
curl localhost:8080/v1/models \
  -H 'Authorization: Bearer dev-secret-key'
```

用户级限流默认关闭。开启后，网关会使用 `identity_header` 指定的请求头作为用户标识，对 `/v1/chat/completions` 独立执行 token bucket 限流；缺少用户标识会返回 `401`，超出限流会返回 `429` 并携带 `Retry-After`：

```json
{
  "rate_limit": {
    "user": {
      "enabled": true,
      "identity_header": "X-User-ID",
      "requests_per_second": 2,
      "burst": 4
    }
  }
}
```

如果同时启用 API Key 鉴权和用户级限流，限流会优先使用鉴权身份中的 `user_id`；未启用鉴权时才回退到 `identity_header`。

聊天补全全局并发限制默认关闭。开启后，网关会限制同时处理中的 `/v1/chat/completions` 请求数量；超过 `max_in_flight` 会返回 `429` 并携带 `Retry-After`。`/v1/models`、健康检查和指标接口不受影响：

```json
{
  "rate_limit": {
    "concurrency": {
      "enabled": true,
      "max_in_flight": 100
    }
  }
}
```

Token 用量预算默认关闭。开启后，网关会对 `/v1/chat/completions` 按身份维护固定时间窗口预算：请求进入时先按 prompt 估算和 `max_tokens` / `max_completion_tokens` 预留 token，响应完成后优先使用 OpenAI `usage.total_tokens` 修正；流式响应没有 usage 时会按 delta 文本估算输出 token。预算不足会返回 `429` 并携带 `Retry-After`：

```json
{
  "token_usage": {
    "enabled": true,
    "identity_header": "X-User-ID",
    "window": "24h",
    "default_budget_tokens": 100000,
    "default_max_completion_tokens": 1024,
    "user_budgets": {
      "alice": 200000
    }
  }
}
```

如果同时启用 API Key 鉴权，token 预算会优先使用鉴权身份中的 `user_id`；未启用鉴权时才回退到 `identity_header`。`default_max_completion_tokens` 用于客户端未显式传入最大输出 token 时的保守预留，以避免并发请求突破预算。

模型代理支持上游超时、按后端熔断和按模型链路降级：

```json
{
  "ai": {
    "request_timeout": "30s",
    "first_token_timeout": "10s",
    "circuit_breaker": {
      "enabled": true,
      "failure_threshold": 3,
      "open_timeout": "30s",
      "half_open_max_requests": 1
    },
    "fallbacks": {
      "gpt-4o-mini": ["gpt-4.1-mini", "mock-b"]
    }
  }
}
```

触发降级的失败包括上游超时、网络错误、`5xx` 和 `429`；普通客户端错误类 `4xx` 会直接透传，不计入熔断失败。流式请求只会在响应写给客户端前降级，首个内容 token 已经输出后不会切换备用模型。

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
