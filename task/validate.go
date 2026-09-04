// CloudflareST - Cloudflare CDN IP 延迟和速度测试工具
// Copyright (C) 2019-2026 XIU2 <https://github.com/XIU2>
// Copyright (C) 2020-2026 masgzy <https://github.com/masgzy>
//
// 本程序基于 XIU2/CloudflareSpeedTest 修改，使用 GPL-3.0 协议开源。
// 完整许可证文本见项目根目录 LICENSE 文件。
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package task

import (
	"fmt"
	"net/url"
)

// Cloudflare CDN 支持的 HTTP / HTTPS 端口
// 来源：Cloudflare Fundamentals – Network ports（https://developers.cloudflare.com/fundamentals/reference/network-ports/）
var (
	cfHTTPPorts = map[int]struct{}{
		80: {}, 8080: {}, 8880: {}, 2052: {}, 2082: {}, 2086: {}, 2095: {},
	}
	cfHTTPSPorts = map[int]struct{}{
		443: {}, 2053: {}, 2083: {}, 2087: {}, 2096: {}, 8443: {},
	}
)

// ValidateTestURL 校验测速地址（-url）：
// 必须以 http:// 或 https:// 协议前缀开头，且包含主机地址
func ValidateTestURL(raw string) error {
	if raw == "" {
		return fmt.Errorf("测速地址为空，请通过 [-url] 指定（或使用 [-dd] 禁用下载测速）")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("测速地址无效：%v", err)
	}
	switch u.Scheme {
	case "http", "https":
	case "":
		return fmt.Errorf("测速地址缺少协议前缀（需以 http:// 或 https:// 开头）：%s", raw)
	default:
		return fmt.Errorf("测速地址协议不支持（仅限 http/https）：%s", raw)
	}
	if u.Host == "" {
		return fmt.Errorf("测速地址缺少主机地址：%s", raw)
	}
	return nil
}

// PortProtocolMismatch 检测测速端口与 URL 协议是否可能不匹配：
// HTTPS 地址搭配 Cloudflare 的纯 HTTP 端口、或 HTTP 地址搭配纯 HTTPS 端口时，
// 测速请求几乎必然失败（TLS 握手对不上），提前提醒用户确认
func PortProtocolMismatch(port int, rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	switch u.Scheme {
	case "https":
		if _, ok := cfHTTPPorts[port]; ok {
			return true
		}
	case "http":
		if _, ok := cfHTTPSPorts[port]; ok {
			return true
		}
	}
	return false
}
