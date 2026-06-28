# ADR 0001: 使用 pdata 作为内部遥测模型

## 状态

Accepted

## 背景

网关需要同时处理 traces、metrics、logs。自定义 `map[string]any` 模型会丢失 histogram、temporality、resource、scope、trace state、span link 和 Prometheus metadata 等信息。

## 决策

内部遥测数据模型使用 OpenTelemetry Collector 的 `pdata`：

- `ptrace.Traces`
- `pmetric.Metrics`
- `plog.Logs`
- `pcommon`

网关自己的批次结构只附加租户、请求、配置版本、接收时间和来源等 metadata。

## 后果

优点：

- 保留 OTLP 类型语义；
- 方便和 Collector 生态互操作；
- fanout 时可以用 `CopyTo` 隔离分支修改；
- 减少 JSON 编解码和无类型转换。

代价：

- 需要跟随 Collector `pdata` API 的版本演进；
- 单元测试需要构造 typed telemetry，而不是随意拼 map。
