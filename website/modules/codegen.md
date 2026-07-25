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

> 生成器只产出文件，**不写入当前仓库**；路由、菜单、权限和迁移仍需开发者接线（见下）。

## 产物清单

```
server/<module>/model.go       # GORM 模型
server/<module>/store.go       # 列表/详情/增删改（树表含组树；主子表含事务替换）
server/<module>/handlers.go    # HTTP 处理函数
server/<module>/routes.go      # RegisterRoutes() 路由注册
web/src/api/<module>.ts        # axios 接口封装
web/src/pages/<module>/index.tsx  # React 列表页（搜索 + 表格 + 表单弹窗）
menu-<module>.sql              # 菜单种子 SQL（需手工调整 id/parent_id/sort 后执行）
```

生成代码遵循仓内全部惯例（响应信封、分页参数、`tenant_id`、权限码格式），生成即合规。

## 落地四步

1. 解压 zip 到项目对应目录；
2. 在目标服务的路由文件里调用生成的 `RegisterRoutes()`；
3. 播种权限点（`<domain>:<module>:list/create/update/delete`）并挂到 `super_admin`；
4. 调整并执行 `menu-<module>.sql` 建菜单。

## 类型映射

| PostgreSQL | Go | TypeScript |
|------------|----|------------|
| int / bigint / smallint / serial* | `int64` | `number` |
| numeric / decimal / float / double / real | `float64` | `number` |
| bool / boolean | `bool` | `boolean` |
| timestamp* / date / datetime* | `time.Time` | `string` |
| 其余（text / varchar / json…） | `string` | `string` |

字段名转换保留常见缩写可读性（`id → ID`、`url → URL`、`api → API`）。

## 接口速查

| 方法 | 路径 | 权限码 | 用途 |
|------|------|--------|------|
| GET | `/api/v1/codegen/tables` | `system:codegen:list` | 可生成表清单 |
| GET | `/api/v1/codegen/tables/:name/columns` | `system:codegen:list` | 表字段元数据（含类型映射） |
| POST | `/api/v1/codegen/preview` | `system:codegen:generate` | 分文件预览 |
| POST | `/api/v1/codegen/download` | `system:codegen:generate` | 下载 zip |

## 已知限制

- 不支持多表 JOIN 联查生成（主子表模式覆盖一对多场景）；
- 字段不能关联字典表渲染下拉（生成后手工替换为字典组件）；
- `required` 只生成前端校验，后端约束需自行补；
- 生成的 API 不自动播种权限点与菜单——这是刻意设计，落地时按项目规范接线。
