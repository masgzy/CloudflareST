package task

import (
	"net"
	"runtime"
	"syscall"
)

const (
	// Linux socket 选项
	SO_BINDTODEVICE = 25

	// macOS socket 选项
	IP_BOUND_IF   = 0x19
	IPV6_BOUND_IF = 0x19

	// Windows socket 选项
	IP_UNICAST_IF   = 31
	IPV6_UNICAST_IF = 31
)

// getBindInterfaceControl 返回一个网络控制函数，用于绑定到指定的网络接口
// 支持 Linux、macOS、Windows 三平台
func getBindInterfaceControl(ifaceName string) func(network, address string, c syscall.RawConn) error {
	// 获取网络接口信息
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return nil // 接口不存在，返回 nil
	}

	return func(network, address string, c syscall.RawConn) error {
		var setErr error
		c.Control(func(fd uintptr) {
			switch runtime.GOOS {
			case "linux":
				// Linux: 使用 SO_BINDTODEVICE
				setErr = setsockoptString(int(fd), syscall.SOL_SOCKET, SO_BINDTODEVICE, ifaceName)
			case "darwin":
				// macOS: 使用 IP_BOUND_IF / IPV6_BOUND_IF
				setErr = bindDarwin(int(fd), iface.Index, network)
			case "windows":
				// Windows: 使用 IP_UNICAST_IF / IPV6_UNICAST_IF
				setErr = bindWindows(fd, uint32(iface.Index), network)
			}
		})
		return setErr
	}
}

// setsockoptString 封装跨平台的 setsockopt string 操作
func setsockoptString(fd int, level, opt int, val string) error {
	var err error
	switch runtime.GOOS {
	case "windows":
		// Windows: 使用 syscall.SetsockoptString（接受 syscall.Handle）
		err = syscall.SetsockoptString(syscall.Handle(fd), level, opt, val)
	default:
		// Unix: 直接使用 syscall.SetsockoptString
		err = syscall.SetsockoptString(fd, level, opt, val)
	}
	return err
}

// setsockoptInt 封装跨平台的 setsockopt int 操作
func setsockoptInt(fd int, level, opt int, val int) error {
	switch runtime.GOOS {
	case "windows":
		return syscall.SetsockoptInt(syscall.Handle(fd), level, opt, val)
	default:
		return syscall.SetsockoptInt(fd, level, opt, val)
	}
}

// bindDarwin macOS 平台绑定接口
func bindDarwin(fd int, ifIndex int, network string) error {
	switch network {
	case "tcp4", "udp4":
		return setsockoptInt(fd, syscall.IPPROTO_IP, IP_BOUND_IF, ifIndex)
	case "tcp6", "udp6":
		return setsockoptInt(fd, syscall.IPPROTO_IPV6, IPV6_BOUND_IF, ifIndex)
	default:
		err := setsockoptInt(fd, syscall.IPPROTO_IP, IP_BOUND_IF, ifIndex)
		if err != nil {
			return setsockoptInt(fd, syscall.IPPROTO_IPV6, IPV6_BOUND_IF, ifIndex)
		}
		return err
	}
}

// bindWindows Windows 平台绑定接口
func bindWindows(fd uintptr, ifIndex uint32, network string) error {
	// Windows 的 fd 是 syscall.Handle (本质是 uintptr)
	handle := syscall.Handle(fd)
	switch network {
	case "tcp4", "udp4":
		return syscall.SetsockoptInt(handle, syscall.IPPROTO_IP, IP_UNICAST_IF, int(ifIndex))
	case "tcp6", "udp6":
		return syscall.SetsockoptInt(handle, syscall.IPPROTO_IPV6, IPV6_UNICAST_IF, int(ifIndex))
	default:
		err := syscall.SetsockoptInt(handle, syscall.IPPROTO_IP, IP_UNICAST_IF, int(ifIndex))
		if err != nil {
			return syscall.SetsockoptInt(handle, syscall.IPPROTO_IPV6, IPV6_UNICAST_IF, int(ifIndex))
		}
		return err
	}
}
