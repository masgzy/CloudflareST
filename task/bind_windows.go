//go:build windows

package task

import (
	"syscall"
)

// bindInterface Windows 平台绑定接口
func bindInterface(fd uintptr, ifaceName string, ifIndex int, network string) error {
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
