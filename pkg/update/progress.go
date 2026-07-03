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
