# Agent Kernel

> 状态：v0；唯一实现位于 `microservices/services/ai/internal/agentcore`。本文定义稳定边界，具体 Runtime（Hermes、CLI、端侧进程等）只能通过 Adapter 接入。

## 1. 定位与依赖方向

Agent Kernel 是模型、工具协议和 Runtime 中立的服务端执行内核，负责不可变定义版本、可靠运行、步骤与事件账本、检查点、租约、取消和恢复。

依赖方向固定为：

```text
Runtime Adapter ──> Agent Kernel <── Model Adapter
                         ^
                         └──── Tool Adapter
```

约束：

- `agentcore` 不导入 Hermes、OpenAI、Anthropic、MCP 或任何端侧 Runtime 包。
- 模型差异由 `ModelPort` 抹平；工具协议差异由 `ToolRegistry`/Adapter 抹平。
- Hermes 未来只能提交/观察 Kernel Run，不能定义 Kernel 数据模型或状态机。
- v0 只允许服务端登记的只读工具；业务写工具、多 Agent 和 Runtime Adapter 不在本批。

## 2. `agent-spec/v1`

发布版本后不可修改；修改配置必须发布下一版本。

```json
{
  "schema": "agent-spec/v1",
  "name": "知识助手",
  "instructions": "仅依据可用知识回答；无法确认时明确说明。",
  "model_id": 12,
  "tools": ["knowledge_search"],
  "max_steps": 8,
  "timeout_seconds": 120
}
```

不变量：

- `model_id` 绑定平台模型目录中的具体模型，运行期间不随默认模型漂移。
- `tools` 是对服务端登记工具的最小授权，不接受客户端自定义执行地址或 Schema；运行前会校验已发布的工具 Schema 哈希，漂移时拒绝执行。
- `max_steps` 范围 `1..32`，`timeout_seconds` 范围 `10..600`。
- 版本保存规范化 JSON 与 SHA-256；Run 永久绑定该版本。

领域对象：

- `AgentDefinition`：租户内可管理的逻辑 Agent，状态为 `active|disabled`。
- `AgentVersion`：不可变 `agent-spec/v1` 快照。
- `ToolDefinition`：代码登记的服务端工具说明、输入 Schema、只读属性和超时。
- `ToolGrant`：版本对工具名的授权；v0 由版本的 `tools` 集合表达。

## 3. Run、Step 与检查点

### 3.1 Run 状态机

```text
queued ───────> running ───────> succeeded
  │                │  └────────> failed
  │                └──> cancelling ──> cancelled
  └────────────────────────────> cancelled
```

- 终态：`succeeded|failed|cancelled`，不可回退。
- 创建 Run 使用 `(tenant_id, requested_by, idempotency_key)` 唯一键；同键同请求（定义ID、trim后的input）返回原 Run，不同请求返回冲突。最新版本是服务端隐式选择，不参与重试冲突判定；后续发布或禁用不影响已提交请求的复用，禁用只拒绝真正新建。存量request hash保留原值，不回写账本。
- `cancel` 幂等；queued Run 可直接取消，running Run 先进入 `cancelling`。
- 每个 Run 绑定 `lease_owner/lease_expires_at`。Worker 仅通过带条件的 claim 获取所有权，心跳续租；进程崩溃后由其他 Worker 重领。恢复时若总超时已耗尽，失败事务同步关闭仍running的步骤/工具并记录失败事件，不重放调用、不覆盖已成功步骤。

### 3.2 Step

Step 是可审计执行单元：

- `model`：一次模型推理；保存请求摘要、文本响应、Tool Calls、Token 用量与 stop reason。
- `tool`：一次工具调用；保存工具名、参数哈希、结果/错误和耗时。

状态为 `running|succeeded|failed|cancelled`。`(run_id, ordinal)` 唯一，防止恢复时重复落账。

### 3.3 Checkpoint/Resume

每个已完成 Step 与下一轮所需消息在同一数据库事务内写入：

```json
{
  "schema": "agent-checkpoint/v1",
  "next_ordinal": 3,
  "messages": [
    {"role": "user", "content": "……"},
    {"role": "assistant", "tool_calls": []},
    {"role": "tool", "tool_call_id": "call_1", "content": "……"}
  ]
}
```

恢复只从最后一个已提交检查点继续，不从内存猜测状态。模型 HTTP 调用本身不具备跨 Provider 的 exactly-once 保证，崩溃窗口可能产生重复计费；v0 明确承诺的是账本一致、已提交步骤不重放、只读工具重放无副作用。

## 4. `tool-spec/v1`

```json
{
  "schema": "tool-spec/v1",
  "name": "knowledge_search",
  "description": "在当前租户知识库中检索相关片段",
  "read_only": true,
  "timeout_ms": 15000,
  "input_schema": {
    "type": "object",
    "additionalProperties": false,
    "required": ["query"],
    "properties": {
      "query": {"type": "string", "minLength": 1, "maxLength": 2000},
      "top_k": {"type": "integer", "minimum": 1, "maximum": 10}
    }
  }
}
```

`knowledge_search` 复用现有 Knowledge Service，并从 Run 的 `tenant_id` 重建租户上下文。客户端不能传 tenant ID。执行器严格拒绝未知字段，`top_k` 默认 5。

工具调用幂等键绑定 `run + model step + call index + tool name + canonical arguments hash`。已成功调用直接返回持久化结果；未确认调用仅允许只读工具安全重放。未来写工具必须另行提供可证明的外部幂等提交协议，不能复用 v0 假设。

## 5. 模型中立契约

Kernel 只依赖：

```text
Complete(ctx, ModelRequest) -> ModelResponse
```

统一消息角色为 `system|user|assistant|tool`，统一响应包括：

- 文本内容；
- 结构化 Tool Calls（ID、名称、JSON 参数）；
- `input_tokens` / `output_tokens`；
- `stop_reason`。

OpenAI-compatible 与 Anthropic 的请求/响应映射位于 AI Provider Adapter，不能渗入 Kernel。无工具请求可使用 Provider 的普通 Chat；一旦版本授权工具，Provider 必须具备结构化 Tool Call 能力，不能回退到正则解析文本。

## 6. `run-event/v1` 与 SSE

事件信封：

```json
{
  "schema": "run-event/v1",
  "id": 101,
  "type": "step.completed",
  "run_id": 42,
  "step_id": 88,
  "occurred_at": "2026-08-31T10:00:00Z",
  "data": {}
}
```

v0 事件类型：

- `run.queued`、`run.started`、`run.resumed`、`run.cancel_requested`；
- `step.started`、`step.completed`、`step.failed`、`step.cancelled`；
- `tool.started`、`tool.completed`、`tool.failed`、`tool.cancelled`；
- `run.succeeded`、`run.failed`、`run.cancelled`。

事件与对应状态更新同事务提交。SSE 使用数据库事件 ID 作为游标，支持 `after_id`/`Last-Event-ID` 断线续传；事件流是观察通道，不是状态真源。

## 7. API v0

前缀 `/api/v1/ai/agent-kernel`：

- `GET/POST /agents`
- `GET/PATCH /agents/:id`
- `GET/POST /agents/:id/versions`
- `GET /tools`
- `POST /agents/:id/runs`
- `GET /runs`
- `GET /runs/:id?after_id=<事件游标>`：默认首批，事件按ID升序最多200条，返回`events_has_more`与`events_next_cursor`。下一批使用实际返回末条ID，空尾批游标不回退；UI每次只保留当前批，可返回首批，不静默丢弃历史。SSE与该游标语义一致。
- `POST /runs/:id/cancel`
- `GET /runs/:id/events`（SSE）

所有普通查询都由租户 GORM 插件隔离；Worker 的跨租户 Raw SQL claim 必须在 claim 后用返回的 `tenant_id` 重建上下文，后续每次状态写入同时校验 `tenant_id + lease_owner`。

权限：`ai:agent:list`、`ai:agent:write`、`ai:agent:run`。

## 8. 安全与可观测边界

- v0 不接受任意 URL、Shell、MCP Server 或客户端工具实现。
- 输入、模型文本、工具参数/结果均有长度上限；错误对外脱敏，完整诊断只写服务日志。
- Run/Step/Event 是 Agent 审计真源；平台审计中另记录定义、发布、运行、取消等管理动作。
- AI 配额检查与 Usage 记录通过通用 Port 接入现有模型治理，不由 Hermes 服务定义 Kernel 语义。
- SSE 先鉴权再查 Run 归属；不存在与跨租户统一返回 404。

## 9. v0 明确不做

Hermes Adapter、多 Agent 协商、拖拽编排、长期记忆、技能市场、任意 MCP/HTTP/Shell 工具、业务写工具、自动重试策略编辑器、跨地域调度，以及把开发期 `.pi` Harness 直接产品化。
