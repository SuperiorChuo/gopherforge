package system

import "testing"

// 移动端 UA 会同时命中桌面关键字，判定顺序错了就会把手机记成桌面系统：
// iPhone 的 UA 含 "like Mac OS X"，Android 的 UA 含 "Linux"。生产库里
// 真机 iPhone 登录曾被记成 macOS（login_logs 有实证），故这里逐条钉死。
func TestParseUserAgentOS(t *testing.T) {
	cases := []struct {
		name    string
		ua      string
		wantOS  string
		wantDev string
	}{
		{
			name:    "iPhone Safari 不是 macOS",
			ua:      "Mozilla/5.0 (iPhone; CPU iPhone OS 27_0_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1",
			wantOS:  "iOS",
			wantDev: "Mobile",
		},
		{
			name:    "iPad 不是 macOS",
			ua:      "Mozilla/5.0 (iPad; CPU OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/604.1",
			wantOS:  "iOS",
			wantDev: "Tablet",
		},
		{
			name:    "Android 不是 Linux",
			ua:      "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36",
			wantOS:  "Android",
			wantDev: "Mobile",
		},
		{
			// 真机 iPad 的 UA 带 "Mobile/15E148"，别被判成手机
			name:    "带 Mobile 标记的 iPad 仍是平板",
			ua:      "Mozilla/5.0 (iPad; CPU OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1",
			wantOS:  "iOS",
			wantDev: "Tablet",
		},
		{
			name:    "真 macOS 仍是 macOS",
			ua:      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			wantOS:  "macOS",
			wantDev: "Desktop",
		},
		{
			name:    "真 Linux 桌面仍是 Linux",
			ua:      "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			wantOS:  "Linux",
			wantDev: "Desktop",
		},
		{
			name:    "Windows",
			ua:      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			wantOS:  "Windows",
			wantDev: "Desktop",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			device, os, _ := parseUserAgent(tc.ua)
			if os != tc.wantOS {
				t.Errorf("os = %q, want %q", os, tc.wantOS)
			}
			if device != tc.wantDev {
				t.Errorf("device = %q, want %q", device, tc.wantDev)
			}
		})
	}
}

func TestParseUserAgentBrowser(t *testing.T) {
	cases := []struct {
		name string
		ua   string
		want string
	}{
		{
			name: "Edge 不是 Chrome",
			ua:   "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 Edg/120.0.0.0",
			want: "Edge",
		},
		{
			name: "iOS 上的 Chrome 是 CriOS",
			ua:   "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/120.0.0.0 Mobile/15E148 Safari/604.1",
			want: "Chrome",
		},
		{
			name: "iOS 上的 Firefox 是 FxiOS",
			ua:   "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) FxiOS/120.0 Mobile/15E148 Safari/604.1",
			want: "Firefox",
		},
		{
			name: "桌面 Safari",
			ua:   "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15",
			want: "Safari",
		},
		{
			name: "Chrome",
			ua:   "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			want: "Chrome",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, browser := parseUserAgent(tc.ua); browser != tc.want {
				t.Errorf("browser = %q, want %q", browser, tc.want)
			}
		})
	}
}

// 脚本与空 UA 落 Unknown 是正确行为（curl/node 登录本就没有浏览器），
// 但要和"解析失败"区分得开：device 仍应给出 Desktop 之外的诚实取值。
func TestParseUserAgentNonBrowser(t *testing.T) {
	for _, ua := range []string{"curl/8.7.1", "node", ""} {
		_, os, browser := parseUserAgent(ua)
		if os != "Unknown" || browser != "Unknown" {
			t.Errorf("ua %q: got os=%q browser=%q, want both Unknown", ua, os, browser)
		}
	}
}
