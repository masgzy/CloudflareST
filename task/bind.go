package task

import (
	"fmt"
	"net"
	"strings"
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

// InterfaceIps 接口 IP 信息
type InterfaceIps struct {
	IPv4 net.IP
	IPv6 net.IP
	Port int
}

// InterfaceParamResult 接口解析结果
type InterfaceParamResult struct {
	InterfaceIPs     *InterfaceIps
	IsValidInterface bool
	InterfaceName    string
	InterfaceIndex   int
}

// processInterfaceParam 解析接口参数（支持 IP、IP:Port、接口名）
func processInterfaceParam(ifaceParam string) (*InterfaceParamResult, error) {
	if ifaceParam == "" {
		return nil, fmt.Errorf("接口参数为空")
	}

	result := &InterfaceParamResult{}

	// 尝试解析为 IP:Port 格式
	if host, port, err := net.SplitHostPort(ifaceParam); err == nil {
		ip := net.ParseIP(host)
		if ip != nil {
			portNum := 0
			if p, err := net.LookupPort("tcp", port); err == nil {
				portNum = p
			}
			result.InterfaceIPs = &InterfaceIps{Port: portNum}
			if ip.To4() != nil {
				result.InterfaceIPs.IPv4 = ip
			} else {
				result.InterfaceIPs.IPv6 = ip
			}
			result.IsValidInterface = true
			return result, nil
		}
	}

	// 尝试解析为纯 IP 格式
	if ip := net.ParseIP(ifaceParam); ip != nil {
		result.InterfaceIPs = &InterfaceIps{}
		if ip.To4() != nil {
			result.InterfaceIPs.IPv4 = ip
		} else {
			result.InterfaceIPs.IPv6 = ip
		}
		result.IsValidInterface = true
		return result, nil
	}

	// 当作接口名处理
	ifaceName := strings.TrimSpace(ifaceParam)
	if ifaceName == "" {
		return nil, fmt.Errorf("接口名不能为空")
	}

	// 验证接口名是否有效
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return nil, fmt.Errorf("无效的网络接口: %s", ifaceName)
	}

	result.InterfaceName = ifaceName
	result.InterfaceIndex = iface.Index
	result.IsValidInterface = true

	return result, nil
}

// ValidateBindInterface 验证绑定接口参数是否有效
// 在程序启动时调用此函数验证用户输入的接口参数
func ValidateBindInterface(ifaceParam string) error {
	if ifaceParam == "" {
		return nil // 空参数是有效的，表示不绑定
	}
	_, err := processInterfaceParam(ifaceParam)
	return err
}

// GetInterfaceIPs 获取接口的 IP 信息（用于 IP 绑定模式）
func GetInterfaceIPs(ifaceParam string) (*InterfaceIps, error) {
	result, err := processInterfaceParam(ifaceParam)
	if err != nil {
		return nil, err
	}
	return result.InterfaceIPs, nil
}

// getBindInterfaceControl 返回一个网络控制函数，用于绑定到指定的网络接口
// 支持 Linux、macOS、Windows 三平台
// 如果接口不存在，返回的函数会在调用时报错
func getBindInterfaceControl(ifaceName string) func(network, address string, c syscall.RawConn) error {
	// 预先验证接口是否存在
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		// 接口不存在，返回一个会报错的函数
		return func(network, address string, c syscall.RawConn) error {
			return fmt.Errorf("无效的网络接口: %s", ifaceName)
		}
	}

	// 接口存在，返回正常的绑定函数
	return func(network, address string, c syscall.RawConn) error {
		var setErr error
		c.Control(func(fd uintptr) {
			setErr = bindInterface(fd, ifaceName, iface.Index, network)
		})
		return setErr
	}
}