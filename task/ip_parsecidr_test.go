// CloudflareST - Cloudflare CDN IP 延迟和速度测试工具
// Copyright (C) 2019-2026 XIU2 <https://github.com/XIU2>
// Copyright (C) 2020-2026 masgzy <https://github.com/masgzy>
//
// 本程序基于 XIU2/CloudflareSpeedTest 修改，使用 GPL-3.0 协议开源。
// 完整许可证文本见项目根目录 LICENSE 文件。

package task

import "testing"

// B-4：parseCIDR 改为返回 error，非法输入不再直接退出进程，可在进程内断言。
// （loadIPRanges 装配层仍快速失败，但错误信息已带文件名/行号上下文。）
func TestParseCIDRInvalid(t *testing.T) {
	for _, bad := range []string{
		"not-an-ip", "1.1.1.1/99", "300.1.1.1", "1.1.1.1/24/24",
		"::/129", "1.1.1.1:99999", "1.1.1.1:abc", "", "  ",
	} {
		r := newIPRanges()
		if err := r.parseCIDR(bad); err == nil {
			t.Errorf("parseCIDR(%q) 期望报错，实际无错", bad)
		}
	}
}

func TestParseCIDRValid(t *testing.T) {
	for _, ok := range []string{
		"1.1.1.1", "104.16.0.0/13", "1.1.1.0/24:443",
		"[2606:4700::/32]:8443", "2606:4700::/32", "::1",
	} {
		r := newIPRanges()
		if err := r.parseCIDR(ok); err != nil {
			t.Errorf("parseCIDR(%q) 意外报错: %v", ok, err)
		}
	}
}
