// CloudflareST - Cloudflare CDN IP 延迟和速度测试工具
// Copyright (C) 2019-2026 XIU2 <https://github.com/XIU2>
// Copyright (C) 2020-2026 masgzy <https://github.com/masgzy>
//
// 本程序基于 XIU2/CloudflareSpeedTest 修改，使用 GPL-3.0 协议开源。
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

package utils

import (
	"fmt"

	"github.com/fatih/color"
)

// 由专业的库来处理多平台的颜色输出效果
var (
	Red     = color.New(color.FgRed)                // 红色 31
	Green   = color.New(color.FgGreen)              // 绿色 32
	Yellow  = color.New(color.FgYellow)             // 黄色 33
	Blue    = color.New(color.FgBlue, color.Bold)   // 蓝色 34
	Magenta = color.New(color.FgMagenta)            // 紫红色 35
	Cyan    = color.New(color.FgHiCyan, color.Bold) // 青色 36
	White   = color.New(color.FgWhite)              // 白色 37
)

// 日志前缀着色（B10）：仅前缀带颜色、正文保持默认色，替代「整行着色 / 纯文本前缀」。
// NO_COLOR 环境变量或非 TTY 场景下，fatih/color 自动退化为纯文本前缀。
var (
	prefixInfo  = color.New(color.FgHiCyan, color.Bold)   // [信息] 青
	prefixWarn  = color.New(color.FgHiYellow, color.Bold) // [警告] 黄
	prefixError = color.New(color.FgRed, color.Bold)      // [错误] 红
	prefixTip   = color.New(color.FgYellow)               // [提示] 黄
	prefixDebug = color.New(color.FgMagenta)              // [调试] 紫红
)

// logPrefixf 输出「彩色前缀 + 正文」的统一入口（正文自动换行，format 末尾无需 \n）
func logPrefixf(p *color.Color, prefix, format string, a ...interface{}) {
	fmt.Println(p.Sprint(prefix), fmt.Sprintf(format, a...))
}

// Info 输出 [信息] 级别日志（青色前缀）
func Info(format string, a ...interface{}) {
	logPrefixf(prefixInfo, "[信息]", format, a...)
}

// Warn 输出 [警告] 级别日志（黄色前缀）
func Warn(format string, a ...interface{}) {
	logPrefixf(prefixWarn, "[警告]", format, a...)
}

// Error 输出 [错误] 级别日志（红色前缀）
func Error(format string, a ...interface{}) {
	logPrefixf(prefixError, "[错误]", format, a...)
}

// Tip 输出 [提示] 级别日志（黄色前缀）
func Tip(format string, a ...interface{}) {
	logPrefixf(prefixTip, "[提示]", format, a...)
}

// Debugf 输出 [调试] 级别日志（紫红色前缀），仅 -debug 模式下由调用方启用
func Debugf(format string, a ...interface{}) {
	logPrefixf(prefixDebug, "[调试]", format, a...)
}
