# 轻量审批流引擎设计方案（BPM Approval Flow）

> 状态：M1 已实现（bpm-service） · 本文档为引擎设计蓝本，随基础设施同步自上游完整版
> 参考产品形态：ruoyi-vue-pro 工作流模块（Flowable + 仿钉钉 SIMPLE 设计器），本方案在 Go 生态自研轻量实现，不引入 Flowable/BPMN 引擎。

> **脚手架适配说明**：本仓只包含引擎本体（`services/bpm/`，业务无关）。文中出现的
> CRM 合同 / 回款审批是上游完整版的首个落地业务场景，仅作设计叙事参考，相关业务代码
> **不在本仓**；业务方接入照 §3.6 / §7 的 `biz_type` + 回调 URL 注册方式自行实现
> （`BPM_CALLBACK_<BIZTYPE>=url`，本仓 compose 默认不注册任何回调目标）。
> 另：本仓不含 notify-service，站内信提醒在未配置 `NOTIFY_API_BASE` +
> `NOTIFY_INTERNAL_TOKEN` 时静默跳过，不阻断审批主流程。

---

## 0. 术语约定

| 术语 | 英文 / 表名前缀 | 说明 |
| --- | --- | --- |
| 流程定义 | definition / `bpm_process_definition` | 一张审批流的"模板"，含版本、节点树 JSON |
| 流程实例 | instance / `bpm_process_instance` | 一次具体审批发起（一份合同发起一次即一条实例） |
| 任务 | task / `bpm_task` | 审批节点在实例中落地的待办条目，一个审批节点可能拆成多个会签任务 |
| 节点 | node | 定义里的一个环节：发起 / 审批 / 抄送 / 条件分支 |
| 审批人规则 | assignee rule | 节点上"谁来审"的解析策略 |
| 会签 / 或签 / 依次 | AND / OR / SEQ | 多审批人时的计数规则 |

全文金额一律以「分」（int64）表述，与 CRM `amount_cents` 惯例一致。

---

## 1. 目标与非目标

### 1.1 目标（M1 总目标）
- 提供一个**与业务解耦**的通用审批流引擎：业务方（CRM 等）只需声明"发起审批 / 监听终态回写"，无需关心审批链路细节。
- 支持仿钉钉的**线性节点树**审批：发起节点 → 若干审批节点（可含条件分支）→ 抄送节点 → 结束。
- 审批人规则覆盖：指定用户、指定角色、部门主管、发起人自选。
- 多审批人支持：会签（全部同意）、或签（一人同意）、依次（按顺序逐个）。
- 审批动作覆盖：同意、拒绝、转办、退回（退回发起人 / 退回上一节点）、撤销。
- 抄送、超时提醒（挂现有 monitor cron + notify 站内信）。
- 终态通过事件回写业务状态（首个对接：CRM 合同 draft→active、回款登记放行）。
- 全链路多租户隔离（`tenant_id`），复用现有数据权限惯例。

### 1.2 非目标（M1 明确不做，避免范围蔓延）
- **不做 BPMN 2.0**：不生成/解析 BPMN XML，不引入 Flowable/Activiti 等重量引擎。
- **不做并行网关 / 包容网关**：M1 只有"排他条件分支"（从上到下取第一个命中，末尾兜底 default）。
- **不做父子流程 / 子流程节点**。
- **不做延迟节点、触发器节点、监听器（HTTP 回调式）**。
- **不做加签 / 减签 / 委派**（转办已覆盖大部分诉求；加减签放到 M3+ 评估）。
- **不做自由画布（拖拽连线）设计器**：只做纵向卡片流的简版配置器。
- **不做表单引擎**：M1 表单由业务方（CRM）自持，引擎只存"发起时的表单快照 JSON"用于展示与条件求值；动态表单权限（字段级读/写/隐藏）留到 M3。
- **不做移动端专属 UI**（复用 Web 响应式）。

---

## 2. 数据模型

### 2.1 表清单总览
| 表 | 用途 | 关键点 |
| --- | --- | --- |
| `bpm_process_definition` | 流程定义 + 版本化 | 同一 `key` 多版本，仅一个 `active` 版本 |
| `bpm_process_instance` | 流程实例 | 指向发起时冻结的定义版本 |
| `bpm_task` | 审批任务（待办） | 审批节点 × 审批人展开；会签多条 |
| `bpm_task_cc`（或 `bpm_cc_record`） | 抄送记录 | 谁在哪个节点被抄送、是否已读 |
| `bpm_process_log`（或 `bpm_activity_log`） | 操作 / 流转日志 | 时间线与流转图数据来源 |

所有表统一含 `tenant_id`（`not null`，逻辑默认 1），并对 `tenant_id` 建索引；与 CRM/identity 现有惯例一致（GORM tag `not null;default:1;index`）。主键统一 `uint64`（对齐 CRM），审批人/用户/部门/角色 ID 均为 `uint64`（对齐 system 用户表虽为 `uint`，跨服务传输统一用 `uint64` 承载，见 §7 归属讨论）。

### 2.2 节点树 JSON Schema（定义的核心）

流程定义的 `node_tree` 字段存一棵**单链 + 条件分支**的节点树。用 TypeScript interface 精确描述（后端以等价的 Go struct + `jsonb` 落库）。

```typescript
// ===== 顶层：一条流程定义的节点树 =====
interface FlowSchema {
  version: number;            // schema 结构版本，便于以后演进
  start: StartNode;           // 唯一发起节点，链的头
}

// 所有节点共享字段
interface BaseNode {
  id: string;                 // 节点唯一 id（前端生成 uuid），流转日志据此定位
  name: string;               // 节点显示名，如"部门经理审批"
  type: 'start' | 'approval' | 'cc' | 'condition';
  next?: AnyNode | null;      // 下一个节点；null/缺省表示到达结束
}
```

```typescript
// ===== 发起节点 =====
interface StartNode extends BaseNode {
  type: 'start';
  // M1 表单由业务方自持；这里仅声明发起时需带的字段 key（用于条件求值/展示）
  formFields?: string[];      // 如 ["amount_cents", "customer_id"]
}

// ===== 审批节点 =====
interface ApprovalNode extends BaseNode {
  type: 'approval';
  assignee: AssigneeRule;     // 审批人规则
  multiMode: 'AND' | 'OR' | 'SEQ'; // 会签 | 或签 | 依次
  // 拒绝时的走向：结束流程（reject）还是退回发起人（back_to_start）
  onReject: 'reject' | 'back_to_start';
  timeoutHours?: number;      // 超时提醒阈值（小时），空=不提醒
  // 依次(SEQ)时是否允许当前人退回上一审批人；会签/或签退回策略见引擎章节
  allowBackPrev?: boolean;
}

// ===== 抄送节点 =====
interface CcNode extends BaseNode {
  type: 'cc';
  targets: AssigneeRule;      // 抄送对象规则（复用审批人规则解析）
}

// ===== 条件分支节点（排他，M1 唯一网关） =====
interface ConditionNode extends BaseNode {
  type: 'condition';
  branches: ConditionBranch[]; // 从上到下取第一个命中；最后一个应为 default
}
interface ConditionBranch {
  id: string;
  name: string;               // 如 "金额 >= 10万"
  expr: ConditionExpr | null; // null 表示 default 兜底分支
  next: AnyNode | null;       // 命中后进入的子链
}

// ===== 条件表达式（M1 只做简单比较 + AND/OR 组合，不做脚本） =====
type ConditionExpr =
  | { op: 'and' | 'or'; items: ConditionExpr[] }
  | { op: 'gt' | 'gte' | 'lt' | 'lte' | 'eq' | 'ne' | 'in';
      field: string;          // 取自发起表单快照，如 "amount_cents"
      value: string | number | Array<string | number>; };

// ===== 审批人规则 =====
interface AssigneeRule {
  // M1 四种；type=self_select 时发起时由发起人指定
  type: 'users' | 'roles' | 'dept_leader' | 'self_select';
  userIds?: number[];         // type=users
  roleIds?: number[];         // type=roles
  // type=dept_leader：以谁的部门为基准取主管
  deptLeaderBase?: 'initiator' | 'form_field';
  deptFormField?: string;     // deptLeaderBase=form_field 时的字段名
  // 找不到候选人时的兜底：自动通过 / 转指定人 / 挂起等管理员处理
  emptyFallback?: 'auto_pass' | 'to_users' | 'suspend';
  fallbackUserIds?: number[]; // emptyFallback=to_users 时
}

type AnyNode = StartNode | ApprovalNode | CcNode | ConditionNode;
```

**Schema 约束（引擎发布定义时校验）：**
1. 有且仅有一个 `start` 节点，且为链头。
2. `condition` 节点的 `branches` 至少 2 个，且**必须有且仅有一个** `expr=null` 的 default 分支（放在数组末尾）。
3. 条件表达式的 `field` 必须出现在 `start.formFields` 声明中（否则发布报错，避免运行期取不到值）。
4. `SEQ`（依次）模式下 `assignee.type` 必须能解析出**有序**候选人列表（`users` 显式顺序 / `roles` 按用户 id 升序）。
5. `self_select` 只允许出现在紧邻发起节点之后的审批节点，或由发起表单显式提供（M1 约束，简化实现）。

> **部门主管的现实约束（重要）**：调研发现 `department` 表的 `Leader` 字段**只是字符串名称，没有 `leader_user_id` 外键**。因此 `dept_leader` 规则在 M1 无法直接落地。二选一（见 §8 开放问题，建议选 A）：
> - **方案 A（推荐）**：给 `department` 增加 `leader_user_id uint`（identity 服务侧的小改动，属基础设施），引擎按 `dept.leader_user_id` 取主管。
> - **方案 B**：M1 先不支持 `dept_leader`，仅放开 `users/roles/self_select` 三种，`dept_leader` 待 A 落地后开启。

### 2.3 建表 DDL 草案（PostgreSQL）

沿用 GORM AutoMigrate 惯例（CRM 无独立迁移目录），下面 DDL 为等价显式表达，便于评审索引与约束。金额分、时间戳、`tenant_id` 惯例与 CRM 对齐。

```sql
-- 流程定义（版本化：同一 key 多行，唯一一个 active 版本）
CREATE TABLE bpm_process_definition (
    id              BIGSERIAL PRIMARY KEY,
    tenant_id       BIGINT      NOT NULL DEFAULT 1,
    key             VARCHAR(64) NOT NULL,          -- 逻辑标识，如 crm_contract_approval
    name            VARCHAR(128) NOT NULL,
    version         INT         NOT NULL DEFAULT 1,
    status          VARCHAR(16) NOT NULL DEFAULT 'draft', -- draft|active|suspended|archived
    node_tree       JSONB       NOT NULL,          -- FlowSchema
    form_schema     JSONB,                         -- 可选：发起表单字段声明（M1 可空）
    biz_type        VARCHAR(32),                   -- 业务类型，如 crm_contract / crm_payment
    remark          VARCHAR(256),
    created_by      BIGINT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX ux_bpm_def_key_ver ON bpm_process_definition (tenant_id, key, version);
CREATE INDEX ix_bpm_def_tenant        ON bpm_process_definition (tenant_id);
CREATE INDEX ix_bpm_def_biz           ON bpm_process_definition (tenant_id, biz_type);
-- 应用层保证：同一 (tenant_id, key) 至多一条 status='active'
```

```sql
-- 流程实例
CREATE TABLE bpm_process_instance (
    id              BIGSERIAL PRIMARY KEY,
    tenant_id       BIGINT      NOT NULL DEFAULT 1,
    definition_id   BIGINT      NOT NULL,          -- 冻结到具体版本行
    definition_key  VARCHAR(64) NOT NULL,          -- 冗余，便于按 key 查询
    title           VARCHAR(256) NOT NULL,         -- 如 "合同审批：XX 项目合同（¥120,000）"
    biz_type        VARCHAR(32) NOT NULL,          -- crm_contract | crm_payment | ...
    biz_id          VARCHAR(64) NOT NULL,          -- 业务对象 id（字符串承载，通用）
    status          VARCHAR(16) NOT NULL DEFAULT 'running',
                    -- running|approved|rejected|canceled
    current_node_id VARCHAR(64),                   -- 当前推进到的节点 id（node_tree 内 id）
    form_snapshot   JSONB       NOT NULL DEFAULT '{}'::jsonb, -- 发起时表单快照（条件求值依据）
    variables       JSONB       NOT NULL DEFAULT '{}'::jsonb, -- 运行期变量（如 self_select 选人结果）
    initiator_id    BIGINT      NOT NULL,
    initiator_dept  BIGINT      NOT NULL DEFAULT 0,
    finished_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ix_bpm_inst_tenant   ON bpm_process_instance (tenant_id, status);
CREATE INDEX ix_bpm_inst_initiator ON bpm_process_instance (tenant_id, initiator_id, status);
CREATE UNIQUE INDEX ux_bpm_inst_biz_running ON bpm_process_instance (tenant_id, biz_type, biz_id)
    WHERE status = 'running';   -- 同一业务对象同时至多一条在途实例（幂等防重）

-- 审批任务（待办）：审批节点按候选人展开；会签一人一条，或签也一人一条（靠计数规则收敛）
CREATE TABLE bpm_task (
    id              BIGSERIAL PRIMARY KEY,
    tenant_id       BIGINT      NOT NULL DEFAULT 1,
    instance_id     BIGINT      NOT NULL,
    node_id         VARCHAR(64) NOT NULL,          -- 定义里的节点 id
    node_name       VARCHAR(128) NOT NULL,
    round           INT         NOT NULL DEFAULT 1, -- 退回重审时同节点的第几轮
    assignee_id     BIGINT      NOT NULL,          -- 当前处理人
    origin_assignee BIGINT,                        -- 转办前的原处理人（NULL=未转办）
    multi_mode      VARCHAR(8)  NOT NULL DEFAULT 'OR', -- AND|OR|SEQ（冗余自节点，便于查询）
    seq_order       INT         NOT NULL DEFAULT 0, -- SEQ 模式下的顺位（0 起）
    status          VARCHAR(16) NOT NULL DEFAULT 'pending',
                    -- pending|approved|rejected|canceled|skipped|returned
                    -- skipped：或签他人先签 / 会签被拒后其余任务作废 / SEQ 未轮到即终止
    comment         VARCHAR(512),                  -- 审批意见
    timeout_at      TIMESTAMPTZ,                   -- 超时提醒时间点（创建时按 timeoutHours 算好）
    reminded_at     TIMESTAMPTZ,                   -- 已发过超时提醒的时间（防重复提醒）
    acted_at        TIMESTAMPTZ,                   -- 实际处理时间
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ix_bpm_task_todo    ON bpm_task (tenant_id, assignee_id, status);      -- 待办列表主查询
CREATE INDEX ix_bpm_task_inst    ON bpm_task (instance_id, node_id, round);
CREATE INDEX ix_bpm_task_timeout ON bpm_task (status, timeout_at) WHERE status = 'pending'; -- 超时扫描

-- 抄送记录
CREATE TABLE bpm_cc_record (
    id              BIGSERIAL PRIMARY KEY,
    tenant_id       BIGINT      NOT NULL DEFAULT 1,
    instance_id     BIGINT      NOT NULL,
    node_id         VARCHAR(64) NOT NULL,
    user_id         BIGINT      NOT NULL,          -- 被抄送人
    read_at         TIMESTAMPTZ,                   -- NULL=未读
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ix_bpm_cc_user ON bpm_cc_record (tenant_id, user_id, read_at);
CREATE INDEX ix_bpm_cc_inst ON bpm_cc_record (instance_id);

-- 操作 / 流转日志（时间线与流转图的数据来源）
CREATE TABLE bpm_process_log (
    id              BIGSERIAL PRIMARY KEY,
    tenant_id       BIGINT      NOT NULL DEFAULT 1,
    instance_id     BIGINT      NOT NULL,
    node_id         VARCHAR(64),                   -- 系统级动作（发起/撤销/终态）可为空
    task_id         BIGINT,                        -- 关联任务（若有）
    action          VARCHAR(32) NOT NULL,
                    -- submit|approve|reject|transfer|return_start|return_prev|cancel|cc|
                    -- timeout_remind|auto_pass|finish_approved|finish_rejected
    operator_id     BIGINT      NOT NULL DEFAULT 0, -- 0=系统
    detail          JSONB,                          -- 附加信息：意见、转办目标、退回目标等
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ix_bpm_log_inst ON bpm_process_log (instance_id, created_at);
```

**设计取舍说明：**
- **实例冻结定义版本**：`instance.definition_id` 指向版本行，定义再发新版不影响在途实例；这是"定义版本化"的全部意义。
- **`biz_id` 用 VARCHAR**：引擎不假设业务主键类型（CRM 是 uint64，未来业务可能是 uuid），通用性优先。
- **部分唯一索引防重**：`ux_bpm_inst_biz_running` 用 PostgreSQL partial unique index 在 DB 层挡住"同一合同重复发起"，比纯应用层校验可靠。
- **`round` 字段**：退回后同一节点重新开任务，用轮次区分历史任务，时间线可完整回放。
- **GORM 落地**：以上 DDL 在实现时转成 model struct + gorm tag（`jsonb` 用 `datatypes.JSON` 或 `[]byte` + 自定义类型），由 AutoMigrate 建表；partial index 需在启动时用 `db.Exec` 补建（AutoMigrate 不支持 WHERE 索引）。

---

## 3. 引擎行为

### 3.1 总体模型：单游标推进的状态机

引擎本质是"**节点树上的单游标 + 任务展开器**"。实例只有一个 `current_node_id` 游标（M1 无并行，天然单游标），推进循环如下：

```
advance(instance):
  loop:
    node = 当前游标节点
    switch node.type:
      case start:      游标 = node.next；continue
      case condition:  按 form_snapshot 从上到下求值 branches，
                       进入第一个命中分支的子链头（default 兜底）；continue
      case cc:         解析 targets → 写 bpm_cc_record + 发站内信（bpm.cc 模板）；
                       游标 = node.next；continue
      case approval:   解析 assignee → 候选人列表
                       若为空 → 按 emptyFallback 处理（auto_pass 则视为节点通过 continue；
                                to_users 换兜底人；suspend 则挂起并通知管理员，break）
                       展开 bpm_task（AND/OR：全员各一条 pending；SEQ：仅 0 号位 pending）
                       发待办站内信（bpm.task_assigned）→ break（等待人工动作）
      case null(链尾): 实例终态 approved → 终态回写（§3.6）→ break
```

**并发与一致性**：所有推进在**单个 DB 事务**内完成（更新任务 → 判定节点结果 → 移动游标 → 展开下个节点任务 → 写日志）。对 `bpm_process_instance` 行使用 `SELECT ... FOR UPDATE` 行锁，防止会签两人同时点"同意"导致双推进。M1 无消息队列（全仓惯例是同步 HTTP），推进就是同步函数调用，简单可靠。

### 3.2 条件分支求值

- 输入：`instance.form_snapshot`（发起时冻结的 JSON）。
- 求值器：遍历 `ConditionExpr` 树，叶子做类型宽松比较（JSON number 统一按 float64→再按字段声明转 int64 比较；金额字段恒为 `amount_cents` 整数分）。
- 顺序：`branches` 从上到下，**第一个命中即进入**，与钉钉/ruoyi SIMPLE 语义一致；全不命中走 default（Schema 校验保证 default 必存在，运行期不会无路可走）。
- 求值失败（字段缺失/类型错乱）：实例挂起 + 日志 `detail` 记录原因 + 站内信通知管理员（不静默走 default，避免错批）。

### 3.3 审批动作语义

| 动作 | 允许者 | 行为 |
| --- | --- | --- |
| 同意 approve | 任务 pending 的 assignee | 本任务 → approved；触发节点计数判定（§3.4） |
| 拒绝 reject | 同上 | 本任务 → rejected；按节点 `onReject`：`reject` → 同节点其余 pending 任务置 skipped，实例 → rejected，终态回写；`back_to_start` → 走"退回发起人"流程 |
| 转办 transfer | 同上 | 本任务 `assignee_id` 换成目标人，`origin_assignee` 记原人，任务保持 pending，重发待办通知；日志记录。不改变计数规则 |
| 退回发起人 return_start | 同上（M2 起） | 当前节点所有 pending 任务置 returned；实例游标回到 start.next 之前的"待重新提交"态：实例仍 running，但生成一条发起人的"重新提交"任务（node_id=start，round+1）。发起人可修改表单快照后重新提交 → 从头 advance（所有审批节点 round+1 重新展开），或直接撤销 |
| 退回上一节点 return_prev | 同上（M2 起，且节点 `allowBackPrev=true`） | 当前节点 pending 置 returned；游标回退到上一个 approval 节点（沿链回溯，跳过 cc/condition），该节点 round+1 重新展开任务。条件分支内的"上一节点"= 分支内上一个 approval；若当前是分支内第一个，则退回到 condition 之前最近的 approval，若无则等价退回发起人 |
| 撤销 cancel | 实例发起人 | 仅 `status=running` 且**首个审批节点尚无 approved 任务**时允许（有人已审后需走拒绝/协商，避免审批意见被静默作废——M1 从严，M2 可放开为"任意 running 可撤"由产品定）。实例 → canceled，全部 pending 任务 → canceled，终态回写 |

所有动作写 `bpm_process_log`；同意/拒绝支持附 `comment`（拒绝必填意见，前端强制）。

### 3.4 会签 / 或签 / 依次的计数规则

判定发生在"某任务 approve/reject 落库后、同一事务内"，对象是**当前节点当前 round 的任务集合**：

- **AND（会签）**：
  - 任一 rejected → 节点结果=拒绝（其余 pending → skipped）。
  - 全部 approved → 节点通过，游标前移。
  - 否则继续等待。
- **OR（或签）**：
  - 任一 approved → 节点通过（其余 pending → skipped）。
  - **全部** rejected → 节点结果=拒绝（有人拒但还有人 pending 时继续等，给其余人"救回"的机会；与钉钉或签语义一致）。
- **SEQ（依次）**：
  - 展开时只建 `seq_order=0` 的任务；当前任务 approved → 若还有下一顺位则建下一条 pending 任务（同节点同 round，seq_order+1），全部顺位走完 → 节点通过。
  - 任一 rejected → 节点结果=拒绝（后续顺位不再创建）。

节点结果=拒绝时统一走 §3.3 的 `onReject` 分派。转办不影响计数（换人不换任务）；退回重审后以新 round 的任务集合重新计数。

### 3.5 超时提醒

- **不引入新调度设施**，两条现成路径二选一，M1 推荐 (a)：
  - (a) **服务内 ticker 循环**：与 `crm/cmd/main.go` 的 followup-due 扫描、ticket 的 overdue 扫描完全同构——bpm 服务启动一个每 5 分钟的 goroutine，扫 `bpm_task WHERE status='pending' AND timeout_at <= now() AND reminded_at IS NULL`（命中 `ix_bpm_task_timeout` 部分索引），逐条发站内信并回填 `reminded_at`。
  - (b) monitor 服务的 `ScheduledJob`（robfig/cron）注册动态任务调 bpm 内部接口。M1 不选：多一跳内部调用、多一处配置，收益仅是可视化 cron 管理。
- **通知落地**：调用 notify-service `POST /api/v1/notify/internal/send`（`X-Internal-Token`，惯例同 CRM 的 `notifyclient`），新增模板：
  - `bpm.task_assigned`（新待办）：Vars `{instance_title, node_name, initiator_name}`，`Link=/bpm/todo?taskId={{task_id}}`，`RefType=bpm_task`，`RefID=task_id`。
  - `bpm.task_timeout`（超时提醒）：同上加 `{hours}`。notify 自带"24h 同模板+RefID 去重"，天然防提醒轰炸。
  - `bpm.cc`（抄送）、`bpm.result`（发起人收终态通知）。
- M1 只做"提醒"，**不做超时自动通过/自动转办**（审批责任不可静默转移，列入开放问题）。

### 3.6 终态回写业务（与 CRM 合同状态机对接）

**机制：同步 HTTP 回调（M1） + 预留事件位（M2+）。** 全仓无 NATS/Kafka（调研确认服务间全为同步 HTTP + `X-Internal-Token`），M1 顺势采用回调：

1. 流程定义按 `biz_type` 在 bpm 服务配置回调地址（环境变量/配置，而非存库，避免租户改配置打内网）：
   `CRM_CALLBACK_BASE=http://go-admin-kit-crm:8xxx` → `POST {base}/api/v1/crm/internal/bpm/callback`（`X-Internal-Token`）。
2. 实例到终态（approved/rejected/canceled）时，同事务写日志，事务提交后**异步发回调**（带重试：立即 + 1min + 5min 三次；仍失败则日志告警 + 站内信通知管理员，人工补偿。回调失败不回滚审批终态——审批事实优先，业务侧幂等补偿）。
3. 回调体：`{instance_id, definition_key, biz_type, biz_id, result: "approved|rejected|canceled", form_snapshot, finished_at}`。业务侧按 `(biz_type,biz_id,instance_id)` 幂等处理。

**CRM 合同状态机改造（业务侧，随 M1 一起做但代码在 crm 服务）：**
- `Contract.Status` 枚举扩为：`draft → approving → active | void`（新增 `approving`；`rejected` 不单设状态，回 `draft` 并在合同上记最近审批结果字段或查 bpm 实例）。
- 提交审批：合同页新增"提交审批"动作 → CRM 调 bpm 发起 API（`biz_type=crm_contract, biz_id=合同id, form_snapshot={amount_cents, customer_id, owner_user_id, no, title}`）→ 成功后合同置 `approving`；`approving` 态禁止编辑合同与直接改状态（`handlers.go` UpdateContract 现有的裸状态修改对 `approving/active` 收口）。
- 回调处理：`approved → active`（补 `signed_at` 逻辑由 CRM 定）；`rejected/canceled → draft`。
- **回款审批（M1 第二场景）**：回款表无状态字段，改造为"申请单"模式——新增 `crm_payment_apply`（或给 `crm_payments` 加 `status: pending_approval|confirmed|rejected`，推荐后者，改动小）：登记回款 → `pending_approval` + 发起 bpm；`approved → confirmed`（此后才计入回款统计）；`rejected → rejected` 保留记录。
- 兼容开关：CRM 侧加 `BPM_ENABLE`（env），关闭时保持现状直改状态，灰度可回退。

---

## 4. API 草案

统一遵循全仓惯例：前缀 `/api/v1/bpm`，响应用 `shared/pkg/response` 的 `{code, message, error_code, data}` 与 `PageResponse`，分页参数 `page`/`page_size`，鉴权取网关 `X-Auth-User-ID / X-Auth-Tenant-ID` 头（JWT 兜底），权限码沿用 `{domain}:{resource}:{action}`（如 `bpm:definition:create`）。

### 4.1 管理端（流程定义，权限：bpm:definition:*）
| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/v1/bpm/definitions` | 定义列表（按 key 聚合显示最新版本，含 active 版本号）；参数 `keyword, biz_type, page, page_size` |
| POST | `/api/v1/bpm/definitions` | 新建定义（key, name, biz_type, node_tree）→ version=1, status=draft |
| GET | `/api/v1/bpm/definitions/:id` | 定义详情（含 node_tree） |
| PUT | `/api/v1/bpm/definitions/:id` | 修改 draft 版本（active 版本不可改，需另存新版本） |
| POST | `/api/v1/bpm/definitions/:id/publish` | 发布：Schema 校验（§2.2 约束）→ 该版本 active，同 key 旧 active → archived |
| POST | `/api/v1/bpm/definitions/:id/new-version` | 以某版本为底复制出新 draft 版本（version=max+1） |
| POST | `/api/v1/bpm/definitions/:id/suspend` | 停用（不再允许新发起，在途实例不受影响） |
| GET | `/api/v1/bpm/definitions/keys/:key/active` | 按 key 取当前 active 版本（发起端/业务端用） |

### 4.2 发起端
| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | `/api/v1/bpm/instances` | 发起：`{definition_key, title, biz_type, biz_id, form_snapshot, variables?}`；`self_select` 节点需在 `variables.selected_assignees[node_id]=[]userId` 提供选人。返回实例 id。业务方（CRM 后端）**服务端到服务端调用**（带 `X-Internal-Token` 的 internal 变体 `POST /api/v1/bpm/internal/instances`），前端不直接对 bpm 发起裸调用，保证表单快照由业务后端权威生成 |
| GET | `/api/v1/bpm/instances/my` | 我发起的（参数 `status, page, page_size`） |
| POST | `/api/v1/bpm/instances/:id/cancel` | 撤销（仅发起人，规则见 §3.3） |
| POST | `/api/v1/bpm/instances/:id/resubmit` | 被退回后修改快照重新提交（M2）：`{form_snapshot}` |

### 4.3 任务端（审批人视角）
| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/v1/bpm/tasks/todo` | 我的待办：`assignee_id=当前用户 AND status=pending`；返回含实例标题、发起人、节点名、到达时间、timeout_at |
| GET | `/api/v1/bpm/tasks/done` | 我的已办（approved/rejected/transferred 历史，含转办出去的） |
| GET | `/api/v1/bpm/tasks/:id` | 任务详情（含实例摘要 + form_snapshot + 我可用的动作列表） |
| POST | `/api/v1/bpm/tasks/:id/approve` | `{comment?}` |
| POST | `/api/v1/bpm/tasks/:id/reject` | `{comment}`（必填） |
| POST | `/api/v1/bpm/tasks/:id/transfer` | `{target_user_id, comment?}`（M2） |
| POST | `/api/v1/bpm/tasks/:id/return` | `{to: "start"|"prev", comment}`（M2） |
| GET | `/api/v1/bpm/cc/my` | 抄送我的列表；`POST /api/v1/bpm/cc/:id/read` 标记已读（M2） |

### 4.4 实例端（详情与流转图）
| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/v1/bpm/instances/:id` | 实例详情：基本信息 + form_snapshot + 当前节点 |
| GET | `/api/v1/bpm/instances/:id/timeline` | 时间线：`bpm_process_log` 按时间正序 + 每条附操作人姓名（bpm 服务批量向 identity 换取用户昵称，或前端用现有用户接口自行映射——M1 取后者，少一个内部依赖） |
| GET | `/api/v1/bpm/instances/:id/diagram` | 流转图数据：返回**定义 node_tree + 每个节点的运行时标注**（`node_id → {state: done|doing|todo|skipped, tasks:[{assignee, status, acted_at, comment}]}`），前端据此渲染纵向卡片流的"进度着色"，无需图形布局引擎 |
| GET | `/api/v1/bpm/internal/instances/by-biz` | internal：`?biz_type=&biz_id=` 供业务侧（CRM 详情页）反查在途/历史实例 |

**可见性规则（M1 从简）**：实例详情可见者 = 发起人 ∪ 全部任务参与者 ∪ 被抄送人 ∪ 持 `bpm:instance:query-all` 权限的管理员。不接数据权限 DataScope 插件（审批可见性语义与部门数据范围不同，避免误伤）。

---

## 5. 前端方案（React 19 + AntD 6，代码位于 `microservices/web/src/`）

### 5.1 简版设计器（仿钉钉，纵向卡片流，不做自由画布）

**交互形态**：中央一条自上而下的节点卡片流（发起人卡片 → …… → 结束占位）。
- 每两卡片之间有一个 `+` 按钮，点击弹出"添加审批人 / 添加抄送人 / 添加条件分支"三选一。
- 点击卡片右侧滑出 Drawer 配置面板：审批节点配"审批人规则（四选一的分段控件 + 对应选择器）、多人模式（会签/或签/依次）、拒绝后走向、超时小时数"；抄送节点配抄送对象；发起节点配 `formFields` 声明（M1 由 biz_type 预置，只读展示）。
- 条件分支渲染为卡片流在该处**横向分叉出 N 列子卡片流**（每列头部是条件卡片，可点开配条件：字段下拉 + 操作符 + 值，AND/OR 组），列尾自动汇合回主流；最右列固定为"默认"不可删。分支内允许继续加审批/抄送节点，**不允许嵌套条件分支（M1 约束，M3 放开）**。
- 节点卡片直接内联校验（如审批人未配置标红），发布时前端整树校验 + 后端二次校验。
- 数据结构即 §2.2 `FlowSchema`，前端所见即所存，无 BPMN 转换层。

**技术选型**：纯 React 组件 + CSS（flex/grid）实现卡片流与连线（连线是纵向线段 + 分叉横线，纯 div/svg 画，无需 X6/ReactFlow 这类画布库——不做拖拽连线，引库反而重）。

### 5.2 页面与组件清单

| 页面 | 路由 | 说明 |
| --- | --- | --- |
| 流程定义列表 | `/bpm/definition` | 表格：key/名称/业务类型/当前版本/状态；操作：设计、发布、停用、新版本 |
| 流程设计器 | `/bpm/definition/:id/design` | §5.1 设计器整页 |
| 待办中心 | `/bpm/todo` | Tab：待办 / 已办 / 我发起的 / 抄送我的；行内快捷"同意/拒绝"（弹意见框），点行进详情 |
| 发起页 | （M1 无独立发起页） | M1 发起入口在**业务侧**：CRM 合同详情页"提交审批"按钮；通用发起页（选流程→填表单）留 M3（依赖表单引擎） |
| 实例详情 | `/bpm/instance/:id` | 上：表单快照只读卡片；中：流转图（复用设计器卡片流组件的只读态 + 进度着色）；下/右：时间线（AntD Timeline：动作、人、意见、时间）；顶部按当前用户动态渲染动作按钮（同意/拒绝/转办/退回/撤销） |

| 组件 | 说明 |
| --- | --- |
| `FlowCanvas` | 节点卡片流渲染器，`mode: edit \| readonly`，readonly 态接受 diagram 接口的运行时标注做着色（绿=done/蓝=doing/灰=todo/虚线=skipped） |
| `NodeCard` / `AddNodeButton` / `BranchColumns` | 卡片流三件套 |
| `AssigneeRuleForm` | 审批人规则配置面板（内嵌 `UserSelectModal` / `RoleSelect`，复用现有 `api/system/user.ts`、`role.ts`、`department.ts`） |
| `ConditionEditor` | 条件表达式编辑器（字段/操作符/值 + AND/OR 组合行） |
| `ApprovalActionBar` + `ApprovalCommentModal` | 详情页动作按钮区与意见弹窗 |
| `TaskListTable` | 待办/已办通用表格 |
| `api/bpm/definition.ts`、`api/bpm/task.ts`、`api/bpm/instance.ts` | API client，惯例同 `api/system/*` |

站内信联动：notify 消息 `Link` 直接指向 `/bpm/todo?taskId=xx` 或 `/bpm/instance/:id`，复用现有消息中心跳转机制。

---

## 6. 落地里程碑

### M1 — 最小可用，直连 CRM 合同审批（后端约 6~8 人日，前端约 6~8 人日）
**范围**：定义 CRUD/发布/版本化；节点类型：发起 + 审批（AND/OR）+ 抄送（可配但仅落记录+站内信）；审批人规则：`users` / `roles` / `self_select`（`dept_leader` 视 leader_user_id 决议，见 §8-Q1）；动作：同意/拒绝/撤销；待办通知站内信；终态回调；CRM 合同接入（`approving` 状态 + 提交审批 + 回调回写 + `BPM_ENABLE` 开关）；前端：定义列表、设计器（无条件分支）、待办中心（待办/已办/我发起）、实例详情（时间线+只读卡片流）。
**验收标准**：
1. 管理员配置"合同审批"流程（两级审批，第二级或签）并发布；改版发布后在途实例仍按旧版走完。
2. 销售提交合同审批 → 合同变 `approving` 且不可编辑；审批人收到站内信，待办可见。
3. 两级全同意 → 合同自动 `active`；任一拒绝 → 合同回 `draft`，发起人收到结果通知；发起人在无人审批前可撤销。
4. 同一合同不能重复发起（DB 唯一约束生效）；跨租户互不可见；`BPM_ENABLE=false` 时 CRM 行为与现状完全一致。

### M2 — 抄送完善 / 退回 / 转办 / 超时（约 5~7 人日）
**范围**：抄送我的列表+已读；转办；退回发起人（resubmit 重走）+ 退回上一节点（round 机制）；超时提醒 ticker + `bpm.task_timeout` 模板；回款审批接入（`crm_payments` 加状态）。
**验收标准**：转办后原人已办可见、新人待办可见且计数不变；退回后发起人改表单重提，全链路 round+1 重新展开，时间线完整可回放；配置 2 小时超时的任务到点收到且仅收到一次提醒站内信；回款 `approved` 前不计入回款统计。

### M3 — 条件分支 / 依次审批 / 表单权限（约 6~9 人日）
**范围**：条件分支（设计器分叉 UI + 求值器 + 挂起兜底）；SEQ 依次模式；发起节点表单字段权限（读/写/隐藏，供通用发起页）；通用发起页（脱离业务侧入口独立发起）；`dept_leader` 规则补全（若 M1 未做）。
**验收标准**：按合同金额 ≥10 万走总监加签链、<10 万走短链，边界值与 default 分支均正确；依次模式严格按序出任务，中途拒绝后续不出；无命中字段时实例挂起且管理员收到通知而非错走分支。

> 粗估口径：1 人日 = 一名熟悉本仓的工程师全职一天，含自测不含联调排期；三阶段合计约 17~24 人日。

---

## 7. 归属判定：引擎放哪个服务？

**结论：新建独立 `microservices/services/bpm/` 服务（bpm-service），不塞进 system。CRM 接入代码留在 crm 服务内。**

按上游「基础设施 vs 业务」规范（基础设施改动同步下游脚手架，业务域永不同步）论证：

1. **引擎是基础设施**：审批流与 RBAC、通知中心同级——任何业务域（CRM、工单、未来的费用报销）都可能挂审批。它必须能进脚手架下游，因此**引擎代码里不允许出现任何 CRM 类型、表名、状态枚举**。
2. **为什么不塞进 system**：system 目前职责是菜单/字典/配置/公告等"轻配置态"能力，而 bpm 有独立且不小的数据模型（5 张表）、后台 ticker、内部回调出口，塞入会让 system 变成杂物间；且下游 monolith 形态合并时，独立包边界（`services/bpm/`）比"system 内一坨"更容易以模块形式并入单进程。微服务形态独立部署，单体形态作为一个 Go module/package 编入，同一套代码两种形态，正是脚手架同步想要的形状。
3. **切割线（怎么切干净）**：
   - **bpm 服务（基础设施，同步下游）**：全部 5 张表、引擎推进逻辑、定义/任务/实例 API、超时 ticker、notify 模板调用、`biz_type + biz_id + 回调 URL` 这一层**字符串化的业务锚点抽象**。回调目标通过环境变量注册（`BPM_CALLBACK_<BIZTYPE>=url`），引擎对"crm_contract"只是一个不透明字符串。
   - **crm 服务（业务，不同步下游）**：`approving` 状态、提交审批按钮的后端动作（调 bpm internal 发起）、`/api/v1/crm/internal/bpm/callback` 回调处理器、回款状态改造、`BPM_ENABLE` 开关。
   - **前端**：`pages/bpm/*` 与 `api/bpm/*` 属基础设施随下游同步；CRM 合同页的"提交审批/审批进度"入口属业务不同步。下游脚手架里 bpm 自带的演示场景可用一个内置 demo biz_type（如 `demo_leave` 请假）替代 CRM，保证脚手架开箱可玩且不携带业务。
   - **identity 侧小改**（若做 leader_user_id）：属基础设施，需同步下游。

---

## 8. 风险与开放问题

### 风险
| # | 风险 | 缓解 |
| --- | --- | --- |
| R1 | **回调丢失导致业务态卡死**（合同永远 `approving`）：同步 HTTP 回调 + 无消息队列，bpm 或 crm 任一侧重启窗口可能丢回调 | 三次重试 + 失败告警（§3.6）；CRM 侧提供管理员"手动同步审批结果"入口（反查 by-biz 接口）；M2+ 评估落一张 `bpm_callback_outbox` 表做可靠投递 |
| R2 | **会签并发双推进**：两人同时点同意 | 实例行 `FOR UPDATE` 锁 + 单事务推进（§3.1），并对任务状态做 `WHERE status='pending'` 条件更新（乐观兜底） |
| R3 | **定义被删/改导致在途实例失稳** | 实例冻结 `definition_id`；active 版本禁编辑；定义只允许 suspend/archive 不允许物理删除（有实例引用时） |
| R4 | **审批人解析结果为空**（角色无人/主管缺失/用户已禁用） | 节点级 `emptyFallback` 三策略（§2.2），默认 `suspend` + 管理员通知，绝不静默跳过 |
| R5 | **ID 类型不一致**：identity/system 用户主键 `uint`，CRM/bpm 用 `uint64` | bpm 全部以 `uint64/BIGINT` 承载，跨服务只传数值 JSON，不共享 Go 类型；文档化约定即可 |
| R6 | **notify-service 在建（另一会话）**：接口形态可能微调 | bpm 侧仿 CRM 建独立 `notifyclient` 薄封装，仅依赖 `internal/send` 契约；通知失败不阻断审批主流程（与现有惯例一致） |
| R7 | **范围蔓延**：ruoyi 功能面大（加签/委派/监听器/表达式），容易被对标裹挟 | 非目标清单（§1.2）+ 里程碑闸门，新诉求一律进 M3+ 评审 |

### 开放问题（实施前需拍板）
- **Q1｜部门主管数据从哪来**：是否接受给 `department` 表加 `leader_user_id`（identity 小改，需同步下游）？若否，M1 直接砍掉 `dept_leader` 规则。**建议：加字段，M1 一并做。**
- **Q2｜撤销的宽严**：M1 取"首个节点无人审过才可撤"；产品上是否要放开为"终态前任意时刻可撤"？
- **Q3｜超时是否升级为自动动作**（自动通过/自动转上级）：涉及审批责任归属，默认只提醒，待业务方明确诉求。
- **Q4｜回款审批的载体**：给 `crm_payments` 加 status（推荐，改动小）还是独立申请单表？若历史回款数据要区分"未走审批的存量"，加 status 方案需给存量补 `confirmed`。
- **Q5｜monolith 下游的表建法**：单体形态下 bpm 表与其他模块同库，AutoMigrate 归属哪个启动序列，随脚手架同步时确认。
- **Q6｜审批权限点粒度**：待办/审批动作是否需要权限码控制（人人可用自己的待办 vs 挂 `bpm:task:*`）？建议任务动作只校验 assignee 身份不设权限码，管理端才挂权限码。

---

## 附：调研事实索引（写作依据，均为对上游完整版代码的只读调研；所引业务服务路径不在本仓）

- CRM 合同/回款模型与裸状态修改：`microservices/services/crm/internal/model/models.go`（Contract L130-146 / Payment L150-163）、`internal/api/handlers.go`（UpdateContract L511-557、状态白名单 L536-541、AddPayment L589-615）、`internal/store/store.go`（AutoMigrate L33-41、回款前置校验 L589-591）。
- notify 发送契约：`microservices/services/notify/internal/api/handlers.go`（internalSendReq L116-130）、`internal/model/models.go`（Template L5-22 / Message L24-41）；调用方样板 `services/crm/internal/notifyclient/client.go`、`services/ticket/cmd/main.go`（overdue ticker 模式）。
- identity/system：用户/角色/部门模型 `microservices/services/system/internal/model/`（Department.Leader 为字符串，无 leader_user_id）；部门树接口 `services/identity/internal/api/system/department.go`；数据权限 `services/identity/internal/pkg/authz/`；响应/分页 `services/shared/pkg/response/`。
- 定时任务：`microservices/services/monitor/internal/service/monitor/job.go`（robfig/cron/v3）。
- 产品对标：ruoyi-vue-pro 工作流文档（doc.iocoder.cn/bpm/，SIMPLE 设计器产 JSON 后转 BPMN 交 Flowable；本方案止步于 JSON，自研推进器）。



