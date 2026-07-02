# LLM 流式请求可观测与容错网关

可以命名为：

> **AegisLLM：面向大模型 API 的流式网关与可观测平台**

不要做成普通“API 网关”，而是专门解决大模型请求的特殊问题。

## 核心功能

第一版只做：

- 提供一个 OpenAI 风格的统一接口
- 转发到两个模拟或真实模型服务
- 支持 SSE 流式输出
- 统计首 Token 延迟、总延迟、请求成功率
- 用户级限流和并发限制
- 超时、熔断和模型降级
- 请求链路追踪
- Token 用量和预算控制
- 简单管理页面

例如：

```
客户端
   ↓
Go AI Gateway
   ├── 鉴权与配额
   ├── 限流
   ├── 路由选择
   ├── SSE 流式代理
   ├── 熔断与降级
   └── Trace / Metrics
        ↓
模型服务 A / 模型服务 B / 本地 Mock 服务
```

## 最有深度的部分

### 1. SSE 流式转发

不能简单地 `io.ReadAll`，而要边读边返回：

```
reader := bufio.NewReader(resp.Body)

for {
    line, err := reader.ReadBytes('\n')
    if len(line) > 0 {
        if _, writeErr := w.Write(line); writeErr != nil {
            return
        }
        flusher.Flush()
    }

    if err != nil {
        break
    }
}
```

可以讲：

- 流式 HTTP
- Flush 机制
- 客户端断开检测
- `context.Context` 取消传播
- 慢客户端造成的背压
- 首字节前和首字节后的重试差异

### 2. 模型降级策略

例如：

```
主模型请求
  ├─ 成功 → 返回
  ├─ 首 Token 前超时 → 重试或切换备用模型
  └─ 已经输出部分 Token → 不能直接重试
```

这比普通网关复杂，因为流已经开始后无法安全重放。

### 3. 限流和配额

至少实现两层：

- 每秒请求数限制
- 同时进行的流式请求数限制

可以自己实现令牌桶，而不是只调用库。

### 4. 可观测性

OpenTelemetry 用于生成、收集和导出 Trace、Metric、Log；Collector 则负责接收、处理和转发遥测数据。

记录：

```
ai.request.duration
ai.first_token.duration
ai.output_tokens
ai.active_streams
ai.upstream.errors
ai.fallback.count
```

## 面试中能讲什么

- Go 如何处理大量长连接
- `goroutine` 是否会随连接数无限增长
- 如何正确关闭 Response Body
- 流式请求为什么不能随便重试
- 限流、熔断、降级的区别
- 如何避免模型服务雪崩
- 如何做可观测性
- 如何压测首 Token 延迟
- 如何处理客户端中途断开
- 如何保证 Token 预算不被并发请求突破

## 范围控制

不要做：

- 完整用户中心
- 充值支付
- 复杂 Agent
- 模型训练
- 大型前端
- 十几个模型厂商适配

只支持两个上游协议即可，其中一个甚至可以是自己写的 Mock 模型服务。