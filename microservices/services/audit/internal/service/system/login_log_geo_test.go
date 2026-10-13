package system

import "testing"

func TestParseLocation(t *testing.T) {
	cases := []struct {
		name     string
		location string
		province string
		city     string
	}{
		{"标准三段", "广东省 深圳市 电信", "广东省", "深圳市"},
		{"无 ISP", "江苏省 南京市", "江苏省", "南京市"},
		{"直辖市", "北京 北京市 联通", "北京", "北京市"},
		{"省加 ISP 无市", "广东省 电信", "广东省", ""},
		{"仅省级", "香港", "香港", ""},
		{"自治州", "新疆维吾尔自治区 伊犁哈萨克自治州 电信", "新疆维吾尔自治区", "伊犁哈萨克自治州"},
		{"内网", "内网", "内网", ""},
		{"ipinfo 内网兜底", "Private Network", "内网", ""},
		{"英文兜底只取省", "Guangdong Shenzhen", "Guangdong", ""},
		{"空串", "", "未知", ""},
		{"纯空白", "   ", "未知", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			province, city := parseLocation(tc.location)
			if province != tc.province || city != tc.city {
				t.Fatalf("parseLocation(%q) = (%q, %q), want (%q, %q)",
					tc.location, province, city, tc.province, tc.city)
			}
		})
	}
}
