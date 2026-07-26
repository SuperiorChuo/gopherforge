# 代码生成器

选表、配字段、一键生成 CRUD 前后端代码——新标准页面分钟级落地。入口：控制台「系统管理 → 代码生成器」（`/system/codegen`）。

## 三种生成模式

| 模式 | 适用 | 约定与产物差异 |
|------|------|----------------|
| **单表** | 普通实体 | 标准列表页 + 弹窗表单 + CRUD API |
| **树表** | 部门/分类等层级数据 | 需指定父级字段（指向自身 id，0 为根）、显示字段、可选排序字段；额外生成 `GET /tree` 全量组树接口；有子节点拒删 |
| **主子表** | 订单-明细类一对多 | 需指定子表与外键字段（自动猜测 `主表名_id` 候选）；创建/更新在**事务内全量替换子表行**（先删后插语义）；子表字段自动推导 |

## 使用流程（三步向导）

1. **选表**：从数据库反射现有表（先建表/写迁移，再生成代码；`goose_db_version` 等系统表自动排除）。
2. **配字段**：填模块名（小写英文，决定路由与目录名）、页面标题、选模式；然后逐字段配置——

   | 配置项 | 说明 |
   |--------|------|
   | 显示名 label | 前端列头与表单标签，默认同列名 |
   | 列表显示 in_list | 是否出现在表格列 |
   | 查询条件 in_search | 是否生成搜索框（默认仅字符串列勾选） |
   | 表单字段 in_form | 是否出现在新建/编辑表单 |
   | 必填 required | 前端表单必填标记（**后端未强制**，需要硬约束自行补） |

   `id / created_at / updated_at / deleted_at` 审计列自动排除。表单控件按字段类型自动推断：数字 → InputNumber、布尔 → Switch、其余 → Input（不支持在向导里自选控件类型）。
3. **预览或下载**：`预览`逐文件查看；`下载`得到 `codegen-<module>.zip`。

## 三种交付方式

| 方式 | 说明 |
|------|------|
| **预览** | 分文件查看将要生成/改动的内容（含对既有接线文件的差异） |
| **下载 ZIP** | 拿走全部产物，自行放入项目 |
| **写入仓库** | 生成器直接把产物落盘并改写接线文件——**默认关闭**，见下方安全说明 |

::: warning 写入仓库是高危能力，默认全关
要启用需**同时**满足四个条件：`CODEGEN_WRITE_ENABLED=true`、`CODEGEN_REPO_ROOT` 指向真实仓库根、调用者是平台管理员、且具备 `system:codegen:write` 权限。路径侧做了绝对化 + 符号链接解析 + 越界拒绝（`..`、绝对路径、卷名、反斜杠、NUL 一律拒），写入走暂存 + 备份 + 加锁。

**仅建议在开发环境启用**；生产环境保持默认关闭，用 ZIP 下载走正常代码评审流程。
:::

镜像内固化了一份**仓库快照**（只含生成器需要读写的接入源文件），所以预览与下载开箱即用，不需要挂载宿主仓库。

## 产物清单

生成的是与本仓一致的**分层结构**，七个文件：

```
microservices/services/system/internal/model/<module>.go          # GORM 模型
microservices/services/system/internal/dao/system/<module>.go     # 数据访问
microservices/services/system/internal/service/system/<module>.go # 业务逻辑
microservices/services/system/internal/api/system/<module>.go     # HTTP 处理
microservices/web/src/api/<module>.ts                             # axios 封装
microservices/web/src/pages/system/<module>/index.tsx             # React 列表页
microservices/services/monitor/migrations/000000_codegen_<module>.sql  # 权限点迁移
```

除新增文件外，生成器还会**改写四处接线**：服务路由注册、前端路由表、侧栏菜单布局、菜单种子。写入模式下自动完成；下载模式下预览里能看到这四处的差异，需自行合并（迁移文件号 `000000` 是占位，落地时改成你仓库的下一个可用号）。

生成代码遵循仓内全部惯例（响应信封、分页参数、`tenant_id`、权限码格式），生成即合规。

## 类型映射

| PostgreSQL | Go | TypeScript |
|------------|----|------------|
| int / bigint / smallint / serial* | `int64` | `number` |
| numeric / decimal / float / double / real | `float64` | `number` |
| bool / boolean | `bool` | `boolean` |
| timestamp* / date / datetime* | `time.Time` | `string` |
| 其余（text / varchar / json…） | `string` | `string` |

字段名转换保留常见缩写可读性（`id → ID`、`url → URL`、`api → API`）。

## 关系与字典

- **多对多**：可声明中间表与两侧外键，生成端会带上关联的读写与前端多选标签。
- **字典绑定**：字段可绑定字典类型，前端自动渲染为下拉而不是纯输入框。
- **主子表**：一对多场景，创建/更新在事务内全量替换子表行。

## 已知限制

- 不支持任意多表 JOIN 联查生成（一对多走主子表、多对多走 m2m 声明，其余需手工）；
- `required` 只生成前端校验，后端硬约束需自行补；
- 权限迁移是模板产物，落地时要把占位的 `000000` 改成你仓库的下一个可用迁移号。

## 接口速查

| 方法 | 路径 | 权限码 |
|------|------|--------|
| GET | `/api/v1/codegen/capabilities` | `system:codegen:list` |
| GET | `/api/v1/codegen/tables` · `/tables/:name/columns` · `/tables/:name/schema` | `system:codegen:list` |
| POST | `/api/v1/codegen/preview` · `/download` | `system:codegen:generate` |
| POST | `/api/v1/codegen/write` | `system:codegen:write` + 平台管理员 + 服务端双开关 |
