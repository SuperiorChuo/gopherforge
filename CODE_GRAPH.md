# CodeGraph 图谱说明

本项目已使用 `@colbymchenry/codegraph` 构建本地代码知识图谱。图谱数据位于 `.codegraph/`，运行工具位于 `.codegraph-tools/`，两者都是本地生成物，已加入 `.gitignore`，不要提交到仓库。

## 当前索引结果

- 索引文件：274
- 图谱节点：4150
- 图谱边：9497
- 数据库大小：7.78 MB
- SQLite 后端：`native`
- 已识别语言：Go、TypeScript、Vue、JavaScript
- 当前额外覆盖：`.mjs`、`.cjs` 脚本已纳入索引

## 常用命令

查看图谱状态：

```powershell
.\.codegraph-tools\node_modules\.bin\codegraph.cmd status .
```

源码变更后增量刷新：

```powershell
.\.codegraph-tools\node_modules\.bin\codegraph.cmd sync .
```

强制重建整套图谱：

```powershell
.\.codegraph-tools\node_modules\.bin\codegraph.cmd index . --force
```

查询符号：

```powershell
.\.codegraph-tools\node_modules\.bin\codegraph.cmd query "HealthAPI" --limit 8
```

按任务构建上下文：

```powershell
.\.codegraph-tools\node_modules\.bin\codegraph.cmd context "梳理登录流程" --max-nodes 20
```

查看已索引文件：

```powershell
.\.codegraph-tools\node_modules\.bin\codegraph.cmd files --path . --format grouped --max-depth 3
```

## 说明

当前没有写入全局 MCP 或 Codex 配置。需要接入编辑器或 Agent 时，可以先在本项目内确认服务能够启动：

```powershell
.\.codegraph-tools\node_modules\.bin\codegraph.cmd serve --mcp
```

确认无误后，再按需要配置对应客户端。
