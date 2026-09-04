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

// htonl 将 32 位值按网络字节序（大端）重排。
// MSDN 要求 IP_UNICAST_IF / IPV6_UNICAST_IF 传入"网络字节序"的接口索引：
// https://learn.microsoft.com/en-us/windows/win32/winsock/ipproto-ip-socket-options
// Windows 实际均为小端平台（amd64/386/arm64），直接字节交换即可。
func htonl(v uint32) uint32 {
	return v<<24 | (v&0xff00)<<8 | (v>>8)&0xff00 | v>>24
}

// bindInterface Windows 平台绑定接口
func bindInterface(fd uintptr, _ string, ifIndex int, network string) error {
	handle := syscall.Handle(fd)
	// 接口索引必须转成网络字节序再传给 setsockopt，否则在小端机器上
	// 索引值字节颠倒，会绑定到错误的接口或直接失败
	ifIndexNet := int(int32(htonl(uint32(ifIndex))))
	switch network {
	case "tcp4", "udp4":
		return syscall.SetsockoptInt(handle, syscall.IPPROTO_IP, IP_UNICAST_IF, ifIndexNet)
	case "tcp6", "udp6":
		return syscall.SetsockoptInt(handle, syscall.IPPROTO_IPV6, IPV6_UNICAST_IF, ifIndexNet)
	default:
		err := syscall.SetsockoptInt(handle, syscall.IPPROTO_IP, IP_UNICAST_IF, ifIndexNet)
		if err != nil {
			return syscall.SetsockoptInt(handle, syscall.IPPROTO_IPV6, IPV6_UNICAST_IF, ifIndexNet)
		}
		return err
	}
}
