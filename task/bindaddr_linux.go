// CloudflareST - Cloudflare CDN IP 延迟和速度测试工具
// Copyright (C) 2020-2026 masgzy <https://github.com/masgzy>
//
// 本程序使用 GPL-3.0 协议开源。
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

//go:build linux

package task

import (
	"net"
	"syscall"
)

// ipBindAddressNoPort 对应 netinet/in.h 中的 IP_BIND_ADDRESS_NO_PORT（Linux 4.16+ 引入）
const ipBindAddressNoPort = 18

// setBindAddressNoPortControl 返回设置 IP_BIND_ADDRESS_NO_PORT 的 Control 回调。
// 效果：bind() 源 IP 时不预先分配临时端口，推迟到 connect() 阶段由内核结合
// 目标地址分配，降低高并发下「源IP:源端口 × 目标」四元组碰撞概率（配合 -intf）。
// 说明：
//   - 选项位于 SOL_IP 层，仅 IPv4 socket 支持，IPv6 目标自动跳过；
//   - 未显式绑定源地址时内核在 connect 阶段一并选路，本选项为 no-op，无副作用；
//   - 旧内核（<4.16）不支持该选项，setsockopt 报错静默忽略，行为退回默认。
func setBindAddressNoPortControl() func(network, address string, c syscall.RawConn) error {
	return func(network, address string, c syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return nil
		}
		if ip := net.ParseIP(host); ip == nil || ip.To4() == nil {
			return nil // 仅 IPv4
		}
		return c.Control(func(fd uintptr) {
			// setsockopt 的错误（如旧内核 ENOPROTOOPT）静默忽略，属于可选优化
			_ = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IP, ipBindAddressNoPort, 1)
		})
	}
}
