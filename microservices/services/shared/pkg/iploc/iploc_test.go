package iploc

import "testing"

func TestLookupIntranet(t *testing.T) {
	cases := []string{
		"127.0.0.1",
		"10.1.2.3",
		"192.168.10.10",
		"172.16.0.1",
		"::1",
		"fe80::1%en0",       // 带 zone 的链路本地
		"192.168.1.1:8080",  // 带端口
		"0.0.0.0",
	}
	for _, ip := range cases {
		if got := Lookup(ip); got != IntranetLabel {
			t.Errorf("Lookup(%q) = %q, want %q", ip, got, IntranetLabel)
		}
	}
}

func TestLookupInvalid(t *testing.T) {
	cases := []string{"", "  ", "not-an-ip", "999.1.1.1", "localhost"}
	for _, ip := range cases {
		if got := Lookup(ip); got != "" {
			t.Errorf("Lookup(%q) = %q, want empty", ip, got)
		}
	}
}

// TestLookupPublic 覆盖两种运行环境：
// 有 xdb 时公网 IP 应查得非空归属地；无 xdb 时（CI/本机未下载）应优雅返回空串。
func TestLookupPublic(t *testing.T) {
	got := Lookup("114.114.114.114")
	if Enabled() {
		if got == "" {
			t.Errorf("xdb 已加载但 Lookup 公网 IP 返回空串")
		}
		t.Logf("114.114.114.114 → %q", got)
	} else {
		if got != "" {
			t.Errorf("xdb 未加载时 Lookup = %q, want empty", got)
		}
		t.Log("xdb 数据文件未部署，验证了降级路径（返回空串）")
	}
}

func TestFormatRegion(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		// 旧版格式：国家|区域|省份|城市|ISP
		{"中国|0|广东省|深圳市|电信", "广东省 深圳市 电信"},
		{"中国|0|北京|北京市|联通", "北京 北京市 联通"},
		{"美国|0|加利福尼亚|洛杉矶|0", "美国 加利福尼亚 洛杉矶"},
		{"中国|0|香港|0|0", "香港"},
		{"0|0|0|内网IP|内网IP", "内网IP"},
		{"0|0|0|0|0", ""},
		{"", ""},
		// 新版格式：国家|省份|城市|ISP|国家码
		{"中国|江苏省|南京市|0|CN", "江苏省 南京市"},
		{"中国|北京|北京市|电信|CN", "北京 北京市 电信"},
		{"United States|California|0|Google LLC|US", "United States California Google LLC"},
	}
	for _, c := range cases {
		if got := FormatRegion(c.in); got != c.want {
			t.Errorf("FormatRegion(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestLookupConcurrent 验证 Searcher 池化后的并发安全（配合 -race 检测）。
func TestLookupConcurrent(t *testing.T) {
	ips := []string{"114.114.114.114", "8.8.8.8", "192.168.1.1", "1.2.4.8"}
	done := make(chan struct{})
	for i := 0; i < 8; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 50; j++ {
				for _, ip := range ips {
					_ = Lookup(ip)
				}
			}
		}()
	}
	for i := 0; i < 8; i++ {
		<-done
	}
}
