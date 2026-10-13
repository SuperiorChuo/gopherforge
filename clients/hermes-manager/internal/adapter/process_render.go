package adapter

import (
	"fmt"
	"strconv"
	"strings"
)

// 本文件刻意不带 build tag：服务定义的渲染与解析都是纯字符串逻辑，放进
// //go:build darwin / windows 里会让对应测试只在该平台跑——而开发机只有一个
// 平台，另一半永远不执行（Windows launcher 的引号续行 bug 就是这样漏过去的）。
// 与平台强相关的只有 launchctl / schtasks 的调用序列，那部分留在各自文件里。

// renderPlist 生成 LaunchAgent。KeepAlive 让 gateway 崩溃后被自动拉回，
// RunAtLoad 覆盖开机/登录自启。HERMES_HOME 指向受管 profile——这是上游的
// profile 边界机制，不改上游源码就能隔离出企业 profile。
//
// 日志路径由调用方拼好传入：渲染函数里用 filepath.Join 会按**宿主**平台选分隔符，
// 在 mac 上渲染 Windows 脚本就会拼出 `C:\logs/gateway.out.log`。
func renderPlist(label, hermesBinary, profileDir, outLog, errLog string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>%s</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
    <string>gateway</string>
    <string>run</string>
  </array>
  <key>EnvironmentVariables</key>
  <dict>
    <key>HERMES_HOME</key><string>%s</string>
  </dict>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>ProcessType</key><string>Background</string>
  <key>StandardOutPath</key><string>%s</string>
  <key>StandardErrorPath</key><string>%s</string>
</dict>
</plist>
`, label, hermesBinary, profileDir, outLog, errLog)
}

// parseLaunchctlRunning 解析 launchctl print 的 state / PID 字段。服务已加载但
// 未在跑（state = waiting）不算 running。
func parseLaunchctlRunning(output string) bool {
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "state = ") {
			return strings.Contains(trimmed, "running")
		}
		if strings.HasPrefix(trimmed, "pid = ") {
			pid, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(trimmed, "pid = ")))
			if err == nil && pid > 0 {
				return true
			}
		}
	}
	return false
}

// renderWindowsLauncher 渲染计划任务实际执行的批处理：设置 HERMES_HOME、记录
// pid、把输出重定向到日志。schtasks 的 /tr 参数没有可靠的多参数与环境变量表达
// 方式，包一层 .cmd 比往 /tr 里塞引号健壮得多。
//
// wmic 取自身 pid 不可靠，改用 powershell 记录子进程 pid：Start-Process
// -PassThru 返回进程对象，Id 写进 pidfile 供 stop/running 用。
//
// powershell 调用必须整条写在一行：cmd 只在引号闭合状态下才把行尾 `^` 当续行
// 转义，`-Command "..."` 的引号一旦跨行打开，后续 `^` 退化成普通字符，命令在
// 那里被截断且引号不配对。宁可长行也不拆行。
func renderWindowsLauncher(hermesBinary, profileDir, outLog, errLog, pidFile string) string {
	return fmt.Sprintf(`@echo off
set HERMES_HOME=%s
powershell -NoProfile -Command "$p = Start-Process -FilePath '%s' -ArgumentList 'gateway','run' -NoNewWindow -PassThru -RedirectStandardOutput '%s' -RedirectStandardError '%s'; Set-Content -Path '%s' -Value $p.Id; Wait-Process -Id $p.Id"
`, profileDir, hermesBinary, outLog, errLog, pidFile)
}
