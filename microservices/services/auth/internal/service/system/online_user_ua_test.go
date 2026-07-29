package system

import "testing"

// 与 audit 的登录日志解析同源的两组坑：移动端 UA 同时含桌面关键字
// （iOS 含 "like Mac OS X"、Android 含 "Linux"），以及现代 Edge 的标记是
// "Edg/" 而不是 "edge"——按 "edge" 匹配会让 Edge 全部落进 Chrome 分支。
func TestParseUserAgentOnlineUser(t *testing.T) {
	cases := []struct {
		name        string
		ua          string
		wantBrowser string
		wantOS      string
	}{
		{
			name:        "iPhone Safari 不是 macOS",
			ua:          "Mozilla/5.0 (iPhone; CPU iPhone OS 27_0_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1",
			wantBrowser: "Safari",
			wantOS:      "iOS",
		},
		{
			name:        "Android Chrome 不是 Linux",
			ua:          "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36",
			wantBrowser: "Chrome",
			wantOS:      "Android",
		},
		{
			name:        "现代 Edge 用 Edg/ 标记，不是 Chrome",
			ua:          "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 Edg/120.0.0.0",
			wantBrowser: "Edge",
			wantOS:      "Windows",
		},
		{
			name:        "iOS 上的 Chrome 是 CriOS",
			ua:          "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/120.0.0.0 Mobile/15E148 Safari/604.1",
			wantBrowser: "Chrome",
			wantOS:      "iOS",
		},
		{
			name:        "桌面 macOS Chrome 不受影响",
			ua:          "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			wantBrowser: "Chrome",
			wantOS:      "macOS",
		},
		{
			name:        "桌面 Linux Firefox 不受影响",
			ua:          "Mozilla/5.0 (X11; Linux x86_64; rv:120.0) Gecko/20100101 Firefox/120.0",
			wantBrowser: "Firefox",
			wantOS:      "Linux",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			browser, os := ParseUserAgent(tc.ua)
			if browser != tc.wantBrowser {
				t.Errorf("browser = %q, want %q", browser, tc.wantBrowser)
			}
			if os != tc.wantOS {
				t.Errorf("os = %q, want %q", os, tc.wantOS)
			}
		})
	}
}
