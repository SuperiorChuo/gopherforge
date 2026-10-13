package adapter

import (
	"strings"
	"testing"
)

// 这些用例不带 build tag，两个平台的服务定义在任意开发机上都会被验证——
// 与 process_render.go 同一个理由：只在目标平台跑的渲染测试等于没有测试。

// LaunchAgent 的三个属性直接决定「进程能不能自愈、日志去哪、profile 隔离对不对」，
// 都是靠 plist 文本表达的，所以钉住渲染结果。
func TestRenderPlistCarriesKeepAliveAndProfileHome(t *testing.T) {
	plist := renderPlist("com.gak.hermes", "/home/versions/v1/.venv/bin/hermes", "/home/profiles/company",
		"/home/logs/gateway.out.log", "/home/logs/gateway.err.log")

	for _, want := range []string{
		"<key>KeepAlive</key><true/>", // 崩溃自愈
		"<key>RunAtLoad</key><true/>", // 登录自启
		"<string>/home/versions/v1/.venv/bin/hermes</string>",
		"<string>gateway</string>",
		"<key>HERMES_HOME</key><string>/home/profiles/company</string>", // 受管 orchestrator profile 隔离
		"/home/logs/gateway.out.log",
	} {
		if !strings.Contains(plist, want) {
			t.Fatalf("plist missing %q:\n%s", want, plist)
		}
	}
}

func TestParseLaunchctlRunning(t *testing.T) {
	cases := []struct {
		name   string
		output string
		want   bool
	}{
		{"running with pid", "state = running\npid = 4213\n", true},
		{"loaded but waiting", "state = waiting\n", false},
		{"pid only", "\tpid = 991\n", true},
		{"empty", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseLaunchctlRunning(tc.output); got != tc.want {
				t.Fatalf("parseLaunchctlRunning(%q) = %v, want %v", tc.output, got, tc.want)
			}
		})
	}
}

func windowsLauncherFixture() string {
	return renderWindowsLauncher(
		`C:\home\versions\v1\.venv\Scripts\hermes.exe`,
		`C:\home\profile`,
		`C:\home\logs\gateway.out.log`,
		`C:\home\logs\gateway.err.log`,
		`C:\home\gateway.pid`,
	)
}

// launcher 脚本决定「计划任务实际跑了什么」，三件事必须对：HERMES_HOME 指向
// 受管 profile、拉起的是 gateway、pid 落盘（stop/running 全靠它）。
func TestRenderWindowsLauncherCarriesProfileAndPID(t *testing.T) {
	script := windowsLauncherFixture()

	for _, want := range []string{
		`set HERMES_HOME=C:\home\profile`,
		`hermes.exe`,
		`'gateway','run'`,
		`C:\home\gateway.pid`,
		`C:\home\logs\gateway.out.log`,
		`Wait-Process`, // 任务实例存活期间等着，schtasks /end 才能结束它
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("launcher missing %q:\n%s", want, script)
		}
	}
}

// cmd 的 `^` 续行在引号打开后失效，跨行写 -Command "..." 会被截断成不配对的
// 引号——计划任务能注册但永远起不来，而且只有真机才看得见。钉住「powershell
// 整条一行、脚本里没有行尾 ^」。
func TestRenderWindowsLauncherKeepsPowerShellOnOneLine(t *testing.T) {
	script := windowsLauncherFixture()

	powershellLines := 0
	for _, line := range strings.Split(script, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasSuffix(line, "^") {
			t.Fatalf("caret continuation is unreliable inside quotes, got line %q", line)
		}
		if !strings.HasPrefix(strings.TrimSpace(line), "powershell ") {
			continue
		}
		powershellLines++
		// 整条命令都在这一行：起进程、写 pid、等待，三段缺一不可
		for _, want := range []string{"Start-Process", "Set-Content", "Wait-Process"} {
			if !strings.Contains(line, want) {
				t.Fatalf("powershell line missing %q: %s", want, line)
			}
		}
		if strings.Count(line, `"`) != 2 {
			t.Fatalf("powershell -Command quotes must be balanced on one line: %s", line)
		}
	}
	if powershellLines != 1 {
		t.Fatalf("want exactly one powershell line, got %d:\n%s", powershellLines, script)
	}
}
