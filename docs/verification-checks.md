# 验证入口与工具链

前置工具：Go（允许按模块的 `go` / `toolchain` 指令选择版本）、Node.js 24、npm、Python 3 和 Bash。微服务目录检查还需要可从 `PATH` 找到的 `rg`（ripgrep）；单体不需要 `rg`。交互式终端里的 shell 函数不等同于脚本可用的可执行文件。

## 本地验证

在前端目录先执行 `npm ci`，然后从仓库根目录运行：

```sh
bash scripts/verify-product.sh checks
bash scripts/verify-product.sh all-go
bash scripts/verify-product.sh frontend
```

- `checks`：执行目录门禁与扫描入口的正负控回归，再检查当前仓库目录。
- `all-go`：构建、`go vet` 和 `go test -race -count=1`。需要当前平台的 C 编译器及可用的 race detector。
- `frontend`：国际化检查、lint、已有 Node 单测及生产构建。

目录门禁返回 `0` 表示通过、`1` 表示布局违规、`2` 表示工具缺失或执行失败。`rg` 的退出码 `1` 仅表示无匹配；其他错误不会再被当作通过。目录枚举与排序失败同样阻断。

CI 的各 Go 服务测试开启 race detector；Monitor 保留 atomic coverage。前端继续运行完整的既有检查，并执行 `npm test`。工程脚本回归与真实目录检查在 Workspace job 中执行。

## npm 依赖审计

Security Scan 审计完整依赖树（包含构建工具等 devDependencies），以 `npm audit --audit-level=high` 阻断高危及以上发现。主仓审计 `microservices/web`；带文档站的发行线使用目录矩阵分别审计 `microservices/web` 与 `website`，缓存绑定各目录自己的 `package-lock.json`，一项失败不取消另一项。`checks` 验证真实锁文件覆盖及审计命令的失败传播，不访问漏洞服务。

## Go 漏洞扫描

从仓库根目录运行全部已跟踪 Go 模块的扫描：

```sh
python3 -B scripts/scan-go-modules.py
bash scripts/verify-product.sh security-go
```

`scan-go-modules.py` 通过 `git ls-files -z` 枚举全部已跟踪的 `go.mod`，默认输出按路径排序的仓库根相对目录 JSON，`--lines` 用于本地循环。新增模块须先加入 Git 索引；未跟踪的临时模块不纳入扫描。Git 执行失败、空清单、缺失或非普通文件、符号链接和越界路径均明确失败，不输出部分清单。该机制独立于 `release-workspace.py`，不改变发行 workspace 的语义。

主仓与 public 扫描 `clients/hermes-manager`、`freeswitch-cc/control-api`、`microservices/services`、`microservices/tools/dbdoc` 共 4 个模块；微服务线扫描后两个模块；单体线扫描 `server`、`tools/dbdoc`，仅提供本地入口。Security Scan 使用同一清单，工作目录、依赖缓存及报告路径均相对仓库根目录；缓存匹配模块内的 `go.*`，覆盖 `go.mod` 与存在的 `go.sum`，兼容纯标准库模块没有 `go.sum` 的情况。CI / Release / Security Scan 的 `GO_VERSION` 统一为 `1.27.4`。 Go CI 固定使用 `golangci-lint v2.14.0`，其分析器支持 Go 1.27.4 的 export data v5；保留原有 lint 规则。

`checks` 包含真实 Git 正负控、扫描退出码传播验证及当前仓库模块枚举，不下载扫描器或访问漏洞数据库。`security-go` 显式执行在线扫描，任何模块返回非零即失败；默认 `all` 不包含它。以下单模块入口仍可用于定向复查。

微服务仓库：

```sh
bash scripts/govulncheck.sh microservices/services
bash scripts/govulncheck.sh microservices/tools/dbdoc
```

单体仓库：

```sh
bash scripts/govulncheck.sh server
bash scripts/govulncheck.sh tools/dbdoc
```

仓库内有其他独立 Go 模块时，对各模块目录执行同一入口。脚本以 `GOWORK=off` 使用指定模块的 `go.mod`，不受父级 workspace 排除或依赖覆盖影响；先在目标模块中执行 `go env GOVERSION`，再用选中的工具链隔离安装 `golang.org/x/vuln/cmd/govulncheck@v1.6.0`，完成后清理临时安装目录。它不会使用或覆盖全局安装的旧扫描器；本地和 Security Scan 工作流共用这个入口。

固定的是分析器版本，漏洞数据库仍按 govulncheck 默认机制在线更新。首次运行可能需要下载工具链或工具依赖；下载、构建、分析失败均返回非零，不能据此声称没有漏洞。

额外参数原样传给扫描器，例如：

```sh
bash scripts/govulncheck.sh microservices/services -format json ./... > govulncheck.json
```

默认文本模式发现可达漏洞时退出码为 `3`。JSON 模式可能返回 `0` 并在报告中包含发现，因此必须解析报告；Security Scan 继续使用既有的第三方依赖符号级可达阻断规则，stdlib 与未确认符号级可达的发现保留为告警。工具链对齐不等于依赖漏洞已修复。
