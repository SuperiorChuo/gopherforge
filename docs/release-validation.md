# 产品发布验证

`bash scripts/verify-product.sh <目标>` 是独立的产品验证入口。仅需要 Git、Go、Node.js/npm、Python 3.9+ 与 Buf，不依赖任何编辑器或 Agent。

- Go 版本以 `microservices/services/go.mod` 为准，Node.js 使用 24。
- 前端先在 `microservices/web` 执行 `npm ci`。
- `all` 不启动 Docker，不读真实环境配置，不执行部署。

| 目标 | 检查内容 |
| --- | --- |
| `release-tools` | 发布矩阵与 SHA/tag 绑定的真实 Git 调用链、workspace 解析、迁移身份与公开内容正负例、依赖缓存路径 |
| `public-migrations` | 仅公开候选：已公开 SQL 的完整文件名、编号、原文不变；拒绝旧缺号补填与编号冲突 |
| `public-hygiene` | 仅公开候选：检查已提交 Git 对象中的内部资料、敏感路径与密钥形态；不读取真实环境文件 |
| `all-go` | workspace 全部服务及公共库、dbdoc、媒体 control-api、Hermes 客户端的 build / vet / test -race |
| `frontend` | i18n、lint、单元测试、TypeScript 与生产构建 |
| `contract` | OpenAPI 与类型重生成无漂移、代码生成编译、冒烟工具、运行配置、WSS 工具、Buf lint / breaking |
| `compose` | 使用示例配置解析 Compose 并核对服务与部署入口；需要 Docker Compose CLI，不启动容器 |
| `all`（默认） | `release-tools` + `all-go` + `frontend` + `contract` |

```bash
bash scripts/verify-product.sh all
bash scripts/verify-product.sh compose
# 显式安装 Chromium 后运行独立的浏览器回归，不使用真实后端：
(cd microservices/web && npx playwright install chromium && npm run test:components && npm run test:demo)
```

`test:components` 使用明确的 HTTP/WebSocket fixture 验证真实 React 通知组件；它不属于静态 Demo 或真实 API 验收。`test:demo` 则拒绝意外的 API/IM 请求与 WebSocket，不以成功 mock 掩盖后端依赖。

任何子步骤失败、工具缺失或目标拼写错误都会返回非零，不会静默跳过。契约检查比较暂存版本，重生成产生差异时需审阅并同步提交生成物。Buf breaking 比较当前 HEAD，不替代跨版本兼容评审。

## Release 的源码身份

手动运行 `.github/workflows/release.yml` 时，必须提供版本 `tag` 和已验收公开候选的完整 40 位 `expected_sha`，`dry_run` 默认 `true`。`expected_sha` 是实际 checkout 的候选 commit，不是工作流所在分支、短 SHA、annotated tag 对象或工作项指纹。若候选由另一源码仓生成，源仓提交与公开候选提交须经完整快照清单另行关联，不能混用两个 SHA。

所有测试、镜像和 Release job 都 checkout 同一 `source_sha` 并核对实际 HEAD；镜像的 OCI revision 与 Release target 也使用它。手动触发的工作流源码 SHA、事件 SHA 可能与候选不同，运行摘要分别记录三者，以及 tree、run ID/attempt。必须核对这些证据，不能只看绿色图标或版本名。

- 演练允许 tag 尚未创建；已有 tag 必须解引用到候选 commit，错误 tag 不因演练而放行。
- 非演练要求已有且匹配的 tag；推镜像前、调用 Release API 前都会重新读取远端 tag，拒绝缺失或漂移。创建 Release 带 `--verify-tag`，不允许从默认分支隐式补 tag。
- tag push 使用事件 commit，同样核对 tag 和实际 checkout；手动触发不能因漏填 SHA 而退化为默认分支。
- 这些读取不是跨 GitHub/GHCR 的原子事务，仍须限制版本 tag 的移动/删除；运行期间出现外部修改时须检查已写入的镜像，不能把后续失败视作“完全没有发布影响”。

`dry_run=false` 会写入版本镜像（正式版本还会更新 `latest`），随后才创建默认草稿 Release；草稿不等于没有外部写入，人工转正是另一步。演练仍实际构建并使用构建缓存。手动触发定义须先存在于公共默认分支；本地回归仅执行工作流 shell、临时真实 Git 和隔离的 Release CLI 替身，不证明 GitHub-hosted 工作流、registry 或发布已运行。

## Go 模块发现

Release 与安全扫描共同使用 `scripts/release-workspace.py`，由 `go work edit -json` 解析 `microservices/go.work`。当前 workspace 包含 `services` 与 `tools/dbdoc`。所有微服务和 `shared` 都属于 `services`，不能再把每个服务目录当成独立 go.mod/go.sum；在 `microservices/tools/dbdoc` 中可直接运行 `go run .`，无需关闭 workspace。

模块目录必须位于 workspace 内，必须存在 go.mod；空模块清单、无效语法、重复模块、越界路径及符号链接逃逸均阻止生成矩阵。媒体 control-api 与 Hermes 客户端等附属模块不属于 workspace，产品全量验证显式以 `GOWORK=off` 单独验证，Dependabot 也单独追踪。

## 公开迁移历史身份

公开候选必须额外执行：

```bash
bash scripts/verify-product.sh public-migrations
```

`scripts/release-migrations.py` 从固定公共提交 `26118eae783e12a3d9f5f24bcae9908a5eaaedcc` 读取 52 份原始 SQL（含 `v0.7.0` 的 51 份及 `000081`），不依赖可移动的 tag。候选必须保留完整公开 Git 历史；CI checkout 应使用 `fetch-depth: 0`。基线对象缺失会失败，检查器不会自动下载或改写任何文件。

历史文件删除、改名、原文变化、重用版本号、补填低于公开最高版本的缺号、SQL 软链均被拒绝；新迁移只能追加更高的唯一编号。检查另一目录可用 `python3 -B scripts/release-migrations.py --directory <候选迁移目录>`，仍从脚本所属仓库读取固定基线。开发源仓的分叉历史预期被拒绝，不能整体覆盖到公开候选；因此 `all` 不自动包含此目标。

此检查不解析新 SQL 的业务语义、不核对运行中数据库的版本表，也不证明数据安全；仍须执行下面的真实演练。不要以身份通过替代升级测试。

## 完整产品前向升级

完整产品候选保留公开的 52 份历史 SQL，新增编号从 `000082` 开始；不要把不同发行线的同号文件互相覆盖。源版本号与发行版本号通过快照回执映射，不能只比较最大版本号。

- PostgreSQL 必须安装 **pgvector**；普通 PostgreSQL 镜像不能运行 AI 建表迁移。示例基础设施使用 `pgvector/pgvector:0.8.6-pg18`。
- 新业务表通过 `ALTER TABLE ... SET SCHEMA` 移动，保留 OID、外键和序列；同一表在两端同时存在时拒绝升级，先人工核对数据。保留旧公开版已有的 `public` 核心兼容副本，不做级联清表。
- 旧公开版 `public.ops_task_runs` 原表移动到 `monitor_svc`，不以少量日志回填代替完整执行历史。序列只能前进，不能倒退到已删除记录的 ID；旧兼容副本仍使用的共享序列也须保留。
- AI seed 使用固定菜单身份 `900–902`。如旧库自定义菜单占用这些 ID，或相同名称/路径对应其他身份，升级会明确拒绝；先备份并人工核对菜单与角色关联，禁止以删除菜单或吞掉冲突绕过。
- 启动默认菜单以唯一的 `name + path` 完整身份匹配存量 ID；没有完整匹配时，仅允许唯一的名称或路径匹配，以保留管理员编辑。旧版 SQL/启动种子可能留下 `Tenant`/`tenant` 同路由双记录，完整身份可区分时保留两条旧行及所有关联，不自动合并/删除。另一个明确的历史兼容项仅限 `setting`/`/system/setting` 的 ID 22（公开 `000005`）与 23（公开 Go 默认集合）这对记录：两行/关联全部保留，新增子项沿用公开 Go 的父 ID 23。不同 ID、第三条重复、其他重复完整身份或无完整匹配的分裂身份会明确拒绝，需要人工核对。不同种子也不得共用同一存量 ID：例如把旧 BPM 的名称改成 `crm` 而保留 `/bpm`，会同时命中两个种子；启动在插入或序列调整之前明确拒绝，保留原菜单及关联，禁止静默合并两棵树。
- 新装先执行迁移，再由各业务服务初始化自管表。仅迁移成功不能证明所有服务已完成初始化。
- 完整产品的追加迁移禁止 `down`，避免“回滚”删掉业务数据。升级前备份数据库、对象存储和配置；回退须停应用并恢复同一时点备份。

`scripts/tests/test-public-upgrade.sh` 可在隔离 Linux Docker 主机运行数据库专项：无宿主端口、无已有数据库挂载、临时数据随容器清理。输入 bundle 必须包含候选 Linux `migrate`、`migration-rehearsal`、`menu-seed-tests`、候选 `migrations/`、公开 `v070/` 与 `public-main/` SQL，并将 `scripts/tests/fixtures/public-menus-26118eae.sql` 复制为 `public-menus-fixture.sql`。菜单 fixture 来自固定公开提交的默认集合，升级前模拟旧启动播种。

菜单测试二进制在 `microservices/services` 下用 `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go test -tags release_integration -c -o <bundle>/menu-seed-tests ./system/internal/service/system` 构建；未提供隔离环境确认与测试 DSN 会失败，不会 skip。

专项记录输入 SHA-256，覆盖空库、两条带数据升级、重复 up、表 OID/外键、删除历史后的序列高水位、三个菜单 ID 冲突拒绝、拒绝 down 后的数据与序列不变。三条路线还实跑菜单模型 AutoMigrate、完整默认集合、四个并发补种、原菜单/自定义编辑与权限关联保留、父子 ID 重映射、重复启动、歧义及跨种子身份复用拒绝。管理员改名负例检查所有菜单/权限关联/外键关联及序列不变，正例额外断言种子到存量 ID 的映射为一对一。其中关系哨兵和菜单模型检查不是完整业务服务启动，仍须补真实 API/worker/UI 验收。

公开候选的 `migration-rehearsal` 使用 `-forward-only`：重复 up、精确匹配 PostgreSQL 的预期拒绝（连接失败/其他 SQL 错误不算通过）、核对迁移账本未变，再次 up。默认开发模式仍要求 tip down/up 成功；CI 根据发行映射选择模式，专项须实跑两个模式。

## 公开内容与推送历史

`bash scripts/verify-product.sh public-hygiene` 检查 **HEAD 的原始 Git 对象**，不是尚未提交的工作区或 index。可用 `python3 -B scripts/release-hygiene.py --revision <完整对象ID>` 检查另一提交。内部开发仓预期拒绝，因此该目标不混入开发源码的 `all`。

CI 的 `Repo Hygiene` job、`scripts/git-hooks/pre-push` 和调用该 hook 的发版预检共用一个检查器。完整产品允许全部业务名与业务 schema，不再把它们当泄漏；公开迁移身份仍由独立目标检查。公开候选须安装 Git hook（`git config core.hooksPath scripts/git-hooks`），不能跳过 hook。

Git 真正调用 hook 时，以 stdin 的待推送对象为准，并检查固定公开基线之后的 **每个提交**。不能靠清理工作区、切换到干净分支、在最后一条提交删除文件来掩盖中间历史泄漏；不源于公开基线的历史也不能 merge 进来。直接无参数调用 hook 时，只检查 HEAD，供发版预检复用。是否允许某个仓库、分支、删除或推送动作仍由发布权限边界决定，卫生通过不授予权限。

检查器拒绝内部记录、真实环境及私钥路径、凭据形态文件名、符号链接和 submodule；敏感路径在读取内容之前拒绝。其他 blob 包括二进制也参与检查，诊断只输出类别和转义路径，不回显命中值。单文件 16MiB、单 tree 256MiB / 20000 路径（含空目录）、一次 push 64 条引用 / 500 条新增提交；缺对象、非法 UTF-8 路径、超限或响应不完整均失败，不自动联网或截断通过。

自动检查范围是提交 tree 中的路径和 blob；提交说明、作者字段、标签说明与引用名称仍须另行审计。密钥形态检测是启发式门禁，不是完整脱敏证明；仍须逐路径白名单与内容审计。测试文件、工作流、示例和检查器自身也不整体豁免密钥检测。静态卫生通过不能替代构建、数据库升级或发布资产验收。

## 完整发布还需要哪些证据

源码验证通过不等于发布成功。正式发布之前，还必须在隔离环境中完成：

1. **全新安装**：从空库迁移、初始化、经网关真实登录与业务冒烟；页面和 API 均可达。
2. **带数据升级**：从上一公开版本建立含用户、权限和业务哨兵数据的数据库，再使用候选迁移升级；核对数据值、租户归属、约束、ID 序列和重复执行。
3. **历史迁移不改写**：已公开的版本号与 SQL 内容是升级契约；不覆盖同号不同义迁移，不通过修改迁移版本表伪造升级完成。
4. **发布资产**：镜像构建、文档站构建、完整服务矩阵及源代码包内容审查；源码验证不会推送镜像或创建 Release。

远程部署入口和数据库备份/恢复要求见 [部署指南](./deployment.md)。运行时验证不得复用开发数据库做破坏性试验。
