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
