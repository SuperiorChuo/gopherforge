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

## Go 漏洞扫描

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
