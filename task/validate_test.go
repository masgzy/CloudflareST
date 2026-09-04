package task

import "testing"

func TestValidateTestURL(t *testing.T) {
	cases := []struct {
		in      string
		wantErr bool
	}{
		{"https://speed.cloudflare.com/__down?bytes=1000000", false},
		{"http://cp.cloudflare.com/cdn-cgi/trace", false},
		{"https://192.168.1.1/file", false},
		{"", true},
		{"speed.cloudflare.com/__down", true}, // 缺少协议前缀
		{"ftp://example.com/file", true},      // 协议不支持
		{"https://", true},                    // 缺少主机
		{"http://", true},                     // 缺少主机
		{"://example.com", true},              // 解析后无协议
	}
	for _, c := range cases {
		err := ValidateTestURL(c.in)
		if c.wantErr && err == nil {
			t.Errorf("ValidateTestURL(%q) 期望报错，实际无错", c.in)
		}
		if !c.wantErr && err != nil {
			t.Errorf("ValidateTestURL(%q) 意外报错: %v", c.in, err)
		}
	}
}

func TestPortProtocolMismatch(t *testing.T) {
	cases := []struct {
		port     int
		url      string
		mismatch bool
	}{
		{443, "https://example.com", false},
		{80, "http://example.com", false},
		{2053, "https://example.com", false},
		{2095, "http://example.com", false},
		{80, "https://example.com", true},   // HTTPS 地址 + HTTP 端口
		{8080, "https://example.com", true}, // HTTPS 地址 + HTTP 端口
		{2095, "https://example.com", true},
		{443, "http://example.com", true},  // HTTP 地址 + HTTPS 端口
		{8443, "http://example.com", true}, // HTTP 地址 + HTTPS 端口
		{2087, "http://example.com", true},
		{8443, "https://example.com", false},
		{12345, "https://example.com", false}, // 自定义端口无法判断，不误报
		{443, "not-a-url", false},             // 无协议时不误报
	}
	for _, c := range cases {
		if got := PortProtocolMismatch(c.port, c.url); got != c.mismatch {
			t.Errorf("PortProtocolMismatch(%d, %q) = %v, want %v", c.port, c.url, got, c.mismatch)
		}
	}
}
