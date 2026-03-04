package task

import (
	"net"
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
			setErr = bindInterface(fd, ifaceName, iface.Index, network)
		})
		return setErr
	}
}