// CloudflareST - Cloudflare CDN IP 延迟和速度测试工具
// Copyright (C) 2020-2026 masgzy <https://github.com/masgzy>
//
// 本程序使用 GPL-3.0 协议开源。
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

package update

import (
	"fmt"
	"sync/atomic"
	"time"
)

// progressBar 极简下载进度条（独立于 utils/progress.go，避免依赖全局单例）
type progressBar struct {
	total      int64
	written    int64
	prefix     string
	lastRender time.Time
}

func newProgressBar(total int64, dst string) *progressBar {
	name := dst
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '/' || name[i] == '\\' {
			name = name[i+1:]
			break
		}
	}
	return &progressBar{total: total, prefix: name}
}

func (p *progressBar) Write(b []byte) (int, error) {
	n := len(b)
	atomic.AddInt64(&p.written, int64(n))
	now := time.Now()
	if now.Sub(p.lastRender) < 200*time.Millisecond {
		return n, nil
	}
	p.lastRender = now
	p.render()
	return n, nil
}

func (p *progressBar) render() {
	w := atomic.LoadInt64(&p.written)
	if p.total <= 0 {
		fmt.Printf("\r  %s  %s", p.prefix, humanBytes(w))
		return
	}
	pct := float64(w) / float64(p.total) * 100
	if pct > 100 {
		pct = 100
	}
	fmt.Printf("\r  %s  %3.0f%%  %s/%s", p.prefix, pct, humanBytes(w), humanBytes(p.total))
}

func (p *progressBar) Finish() {
	p.render()
	fmt.Println()
}

func humanBytes(n int64) string {
	const k = 1024
	if n < k {
		return fmt.Sprintf("%d B", n)
	}
	if n < k*k {
		return fmt.Sprintf("%.1f KB", float64(n)/k)
	}
	if n < k*k*k {
		return fmt.Sprintf("%.1f MB", float64(n)/(k*k))
	}
	return fmt.Sprintf("%.1f GB", float64(n)/(k*k*k))
}
