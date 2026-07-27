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

//go:build windows

package task

import (
	"syscall"
)

// bindInterface Windows 平台绑定接口
func bindInterface(fd uintptr, _ string, ifIndex int, network string) error {
	handle := syscall.Handle(fd)
	ifIndex32 := uint32(ifIndex)
	switch network {
	case "tcp4", "udp4":
		return syscall.SetsockoptInt(handle, syscall.IPPROTO_IP, IP_UNICAST_IF, int(ifIndex32))
	case "tcp6", "udp6":
		return syscall.SetsockoptInt(handle, syscall.IPPROTO_IPV6, IPV6_UNICAST_IF, int(ifIndex32))
	default:
		err := syscall.SetsockoptInt(handle, syscall.IPPROTO_IP, IP_UNICAST_IF, int(ifIndex32))
		if err != nil {
			return syscall.SetsockoptInt(handle, syscall.IPPROTO_IPV6, IPV6_UNICAST_IF, int(ifIndex32))
		}
		return err
	}
}
