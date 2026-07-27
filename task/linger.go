package task

import "syscall"

// chainControl 合并多个 Control 回调（用于同时设置 SO_LINGER 和接口绑定）
// nil 回调会被自动跳过
func chainControl(controls ...func(network, address string, c syscall.RawConn) error) func(network, address string, c syscall.RawConn) error {
	return func(network, address string, c syscall.RawConn) error {
		for _, ctrl := range controls {
			if ctrl == nil {
				continue
			}
			if err := ctrl(network, address, c); err != nil {
				return err
			}
		}
		return nil
	}
}
