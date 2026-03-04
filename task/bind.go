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
				setErr = bindToDeviceLinux(int(fd), ifaceName)
			case "darwin":
				// macOS: 使用 IP_BOUND_IF / IPV6_BOUND_IF
				setErr = bindToInterfaceDarwin(int(fd), iface.Index, network)
			case "windows":
				// Windows: 使用 IP_UNICAST_IF / IPV6_UNICAST_IF
				setErr = bindToInterfaceWindows(int(fd), uint32(iface.Index), network)
			}
		})
		return setErr
	}
}

// bindToDeviceLinux Linux 平台绑定接口
func bindToDeviceLinux(fd int, ifaceName string) error {
	return syscall.SetsockoptString(fd, syscall.SOL_SOCKET, SO_BINDTODEVICE, ifaceName)
}

// bindToInterfaceDarwin macOS 平台绑定接口
func bindToInterfaceDarwin(fd int, ifIndex int, network string) error {
	switch network {
	case "tcp4", "udp4":
		// IPv4: 使用 IP_BOUND_IF
		return syscall.SetsockoptInt(fd, syscall.IPPROTO_IP, IP_BOUND_IF, ifIndex)
	case "tcp6", "udp6":
		// IPv6: 使用 IPV6_BOUND_IF
		return syscall.SetsockoptInt(fd, syscall.IPPROTO_IPV6, IPV6_BOUND_IF, ifIndex)
	default:
		// 默认尝试 IPv4，失败则尝试 IPv6
		err := syscall.SetsockoptInt(fd, syscall.IPPROTO_IP, IP_BOUND_IF, ifIndex)
		if err != nil {
			return syscall.SetsockoptInt(fd, syscall.IPPROTO_IPV6, IPV6_BOUND_IF, ifIndex)
		}
		return err
	}
}

// bindToInterfaceWindows Windows 平台绑定接口
func bindToInterfaceWindows(fd int, ifIndex uint32, network string) error {
	switch network {
	case "tcp4", "udp4":
		// IPv4: 使用 IP_UNICAST_IF
		return syscall.SetsockoptInt(fd, syscall.IPPROTO_IP, IP_UNICAST_IF, int(ifIndex))
	case "tcp6", "udp6":
		// IPv6: 使用 IPV6_UNICAST_IF
		return syscall.SetsockoptInt(fd, syscall.IPPROTO_IPV6, IPV6_UNICAST_IF, int(ifIndex))
	default:
		// 默认尝试 IPv4，失败则尝试 IPv6
		err := syscall.SetsockoptInt(fd, syscall.IPPROTO_IP, IP_UNICAST_IF, int(ifIndex))
		if err != nil {
			return syscall.SetsockoptInt(fd, syscall.IPPROTO_IPV6, IPV6_UNICAST_IF, int(ifIndex))
		}
		return err
	}
}
