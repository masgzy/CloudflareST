//go:build windows

package task

import "syscall"

// setLingerControl 返回设置 SO_LINGER(0) 的 Dialer.Control 回调
// 使 close() 发送 RST 而非 FIN，跳过 TIME_WAIT 状态
// Windows 同样存在 TIME_WAIT（默认 120 秒，比 Linux 更长），也需要 SO_LINGER(0)
func setLingerControl() func(network, address string, c syscall.RawConn) error {
	return func(network, address string, c syscall.RawConn) error {
		return c.Control(func(fd uintptr) {
			_ = syscall.SetsockoptLinger(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_LINGER,
				&syscall.Linger{Onoff: 1, Linger: 0})
		})
	}
}
