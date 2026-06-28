# ADR 0002: CEL 只负责匹配，动作由 Go 实现

## 状态

Accepted

## 背景

策略需要支持按 signal、resource、scope、attributes、tenant、environment 等条件匹配。CEL 适合表达安全、可嵌入、非图灵完备的条件，但不适合直接修改完整 telemetry payload。

## 决策

CEL 只用于匹配条件，例如：

```text
signal == "logs" && resource["service.name"] == "payment"
```

删除、重命名、哈希、脱敏、丢弃、采样和路由等动作全部由 Go action 实现。

## 后果

优点：

- 策略复杂度可控；
- action 可以做静态校验；
- 修改语义一致；
- 性能和审计更容易度量。

代价：

- 每新增一种动作都需要 Go 代码；
- 不能把任意数据改写逻辑下放给 CEL 表达式。
