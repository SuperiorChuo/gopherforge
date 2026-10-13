# gak-hermes-manager

Hermes P1 的员工电脑侧管理核心。它只主动连接 go-admin-kit 控制面，负责：

- 在本机生成 Ed25519 设备密钥并完成 device-code 绑定；
- 对设备请求签名，并拒绝过期、被篡改或签名无效的期望状态；
- 下载控制面批准的钉版制品，校验 SHA-256 后安装并原子切换版本指针；
- 只渲染受管 `company` profile，按需换发企业模型 token；
- 通过 macOS `launchd` 或 Windows 计划任务启停 Hermes gateway；
- 收敛 `active`、`stopped`、`revoked` 状态，发送最小心跳和结构化生命周期事件；
- 更新失败时保留当前版本，并支持切回上一个已安装版本。

控制协议只包含固定 schema，不接收 shell、脚本或任意本机路径。

## 当前准入状态

安装链已在 macOS 完成真实上游制品验证，Windows 执行层已有带标签测试覆盖。以下事项完成前仍不能作为生产安装包发布：

- 设备私钥接入 macOS Keychain / Windows Credential Manager；
- Manager 安装包签名与正式下载流水线；
- Windows 真机验证；
- 完整生命周期 canary：开通、安装、首启、停用、恢复、更新、失败回滚和设备吊销。

## 版本口径

心跳里的 `hermes_version` 上报控制面制品版本，例如 `v2026.7.20`。安装比较、期望状态收敛和回滚都使用这一版本标识；`hermes --version` 输出的上游程序版本仅作为安装健康检查结果，不替代制品版本。

## 开发期使用

```bash
go run ./cmd/gak-hermes-manager enroll --server https://control.example
go run ./cmd/gak-hermes-manager sync
go run ./cmd/gak-hermes-manager run
go run ./cmd/gak-hermes-manager status
go run ./cmd/gak-hermes-manager start
go run ./cmd/gak-hermes-manager stop
go run ./cmd/gak-hermes-manager rollback
```

默认状态目录来自 `os.UserConfigDir()` 下的 `go-admin-kit/hermes-manager`，可用 `GAK_HERMES_HOME` 覆盖。开发期私钥保存在仅当前用户可读的 `device.key`；Windows Credential Manager、macOS Keychain 和 Linux Secret Service 接入是生产安装包的准入项。
