//go:build !windows

package task

import (
	"runtime"
	"syscall"
)

// bindInterface Unix 平台绑定接口
func bindInterface(fd uintptr, ifaceName string, ifIndex int, network string) error {
	switch runtime.GOOS {
	case "linux":
		// Linux: 使用 SO_BINDTODEVICE
		return syscall.SetsockoptString(int(fd), syscall.SOL_SOCKET, SO_BINDTODEVICE, ifaceName)
	case "darwin":
		// macOS: 使用 IP_BOUND_IF / IPV6_BOUND_IF
		return bindDarwin(int(fd), ifIndex, network)
	default:
		return nil
	}
}

// bindDarwin macOS 平台绑定接口
func bindDarwin(fd int, ifIndex int, network string) error {
	switch network {
	case "tcp4", "udp4":
		return syscall.SetsockoptInt(fd, syscall.IPPROTO_IP, IP_BOUND_IF, ifIndex)
	case "tcp6", "udp6":
		return syscall.SetsockoptInt(fd, syscall.IPPROTO_IPV6, IPV6_BOUND_IF, ifIndex)
	default:
		err := syscall.SetsockoptInt(fd, syscall.IPPROTO_IP, IP_BOUND_IF, ifIndex)
		if err != nil {
			return syscall.SetsockoptInt(fd, syscall.IPPROTO_IPV6, IPV6_BOUND_IF, ifIndex)
		}
		return err
	}
}
