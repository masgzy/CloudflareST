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

import "strings"

// isWideRune 判断字符是否为宽字符（终端占 2 列）
// 范围参考 Unicode UAX #11 East_Asian_Width 的 Wide(W)/Fullwidth(F) 分类，
// 覆盖中日韩文字、谚文、全角符号等常见宽字符
func isWideRune(r rune) bool {
	switch {
	case r >= 0x1100 && r <= 0x115F, // Hangul Jamo
		r >= 0x2E80 && r <= 0x303E,   // CJK 部首、康熙部首、注音、CJK 符号（含全角空格 U+3000）
		r >= 0x3041 && r <= 0x33FF,   // 平假名、片假名、CJK 兼容
		r >= 0x3400 && r <= 0x4DBF,   // CJK 扩展 A
		r >= 0x4E00 && r <= 0x9FFF,   // CJK 统一表意文字
		r >= 0xA000 && r <= 0xA4CF,   // 彝文
		r >= 0xA960 && r <= 0xA97F,   // 谚文字母扩展 A
		r >= 0xAC00 && r <= 0xD7A3,   // 谚文音节
		r >= 0xF900 && r <= 0xFAFF,   // CJK 兼容表意文字
		r >= 0xFE10 && r <= 0xFE19,   // 竖排形式
		r >= 0xFE30 && r <= 0xFE6F,   // CJK 兼容形式
		r >= 0xFF00 && r <= 0xFF60,   // 全角 ASCII 及标点
		r >= 0xFFE0 && r <= 0xFFE6,   // 全角符号
		r >= 0x20000 && r <= 0x2FFFD, // CJK 扩展 B~F
		r >= 0x30000 && r <= 0x3FFFD: // CJK 扩展 G~H
		return true
	}
	return false
}

// displayWidth 计算字符串的终端显示宽度（列数）：
// 宽字符按 2 列计，其余按 1 列计；Go 的 fmt 以字节为准，直接用 %-Ns 对齐中文会错位
func displayWidth(s string) int {
	width := 0
	for _, r := range s {
		if isWideRune(r) {
			width += 2
		} else {
			width++
		}
	}
	return width
}

// padDisplay 右侧补空格，使字符串占满 width 个显示列（中日韩等宽字符按 2 列计）
// 字符串本身超宽时原样返回
func padDisplay(s string, width int) string {
	w := displayWidth(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}
