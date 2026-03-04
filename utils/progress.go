package utils

import (
	"fmt"
	"math"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/term"
)

// 进度条配置常量
const (
	progressBarSpeed    = 0.2  // 进度条动画速度
	waveWidth           = 16.0 // 波动宽度
	speedFactor         = 0.3  // 速度因子
	saturationBase      = 0.6  // 基础饱和度
	refreshIntervalMs   = 40   // 刷新间隔（毫秒）
	terminalDefaultWidth = 80  // 默认终端宽度
)

// 进度条亮度范围
var progressBarBrightness = [2]float64{0.5, 0.3}

// globalProgressStop 全局进度条停止标志
var globalProgressStop int32 = 0

// StopAllProgress 停止所有进度条
func StopAllProgress() {
	atomic.StoreInt32(&globalProgressStop, 1)
}

// IsProgressStopped 检查是否已停止
func IsProgressStopped() bool {
	return atomic.LoadInt32(&globalProgressStop) == 1
}

// TextData 文本数据
type TextData struct {
	pos    int
	msg    string
	prefix string
}

// BarInner 进度条内部结构
type BarInner struct {
	text      *TextData
	textMu    sync.RWMutex
	done      bool // 改名为 done
	doneMu    sync.RWMutex
	total     int
	startStr  string
	endStr    string
}

// Bar 进度条
type Bar struct {
	inner     *BarInner
	stopChan  chan struct{}
	startTime time.Time
}

// NewBar 创建新的进度条
func NewBar(count int, startStr, endStr string) *Bar {
	inner := &BarInner{
		text: &TextData{
			pos:    0,
			msg:    "",
			prefix: "",
		},
		done:     false,
		total:    count,
		startStr: startStr,
		endStr:   endStr,
	}

	bar := &Bar{
		inner:     inner,
		stopChan:  make(chan struct{}),
		startTime: time.Now(),
	}

	// 启动渲染协程
	go bar.runRenderLoop()

	return bar
}

// runRenderLoop 渲染循环
func (b *Bar) runRenderLoop() {
	ticker := time.NewTicker(refreshIntervalMs * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-b.stopChan:
			b.renderOnce()
			fmt.Println() // 换行
			return
		case <-ticker.C:
			// 检查全局停止标志
			if IsProgressStopped() {
				b.renderOnce()
				fmt.Println()
				return
			}
			if b.inner.isCompleted() {
				b.renderOnce()
				fmt.Println()
				return
			}
			b.renderOnce()
		}
	}
}

// isCompleted 检查是否完成
func (bi *BarInner) isCompleted() bool {
	bi.doneMu.RLock()
	defer bi.doneMu.RUnlock()
	return bi.done
}

// setCompleted 设置完成状态
func (bi *BarInner) setCompleted(completed bool) {
	bi.doneMu.Lock()
	defer bi.doneMu.Unlock()
	bi.done = completed
}

// renderOnce 执行一次渲染
func (b *Bar) renderOnce() {
	b.inner.textMu.RLock()
	textSnapshot := *b.inner.text
	b.inner.textMu.RUnlock()

	currentPos := textSnapshot.pos
	total := b.inner.total
	if total < 1 {
		total = 1
	}

	// 获取终端宽度
	termWidth := getTerminalWidth()
	reservedSpace := 20 + len(b.inner.startStr) + len(b.inner.endStr) + 10
	barLength := termWidth - reservedSpace
	if barLength < 10 {
		barLength = 10
	}

	// 计算进度
	elapsed := time.Since(b.startTime)
	progress := float64(currentPos) / float64(total)
	if currentPos > total {
		progress = 1.0
	}
	filled := int(progress * float64(barLength))
	phase := math.Mod(elapsed.Seconds()*progressBarSpeed, 1.0)

	// 构建进度条字符串
	barStr := b.buildProgressBar(barLength, filled, progress, phase, elapsed)

	// 构建输出
	output := fmt.Sprintf("\r\x1b[K\x1b[33m%s\x1b[0m %s %s \x1b[32m%s\x1b[0m %s",
		textSnapshot.msg, barStr, b.inner.startStr, textSnapshot.prefix, b.inner.endStr)

	// 输出到终端
	os.Stdout.WriteString(output)
}

// buildProgressBar 构建进度条
func (b *Bar) buildProgressBar(barLength, filled int, progress, phase float64, elapsed time.Duration) string {
	barStr := ""
	unfilledBg := [3]uint8{70, 70, 70} // 灰色背景

	// 百分比文本
	percentContent := fmt.Sprintf(" %4.1f%% ", progress*100)
	percentChars := []rune(percentContent)
	percentLen := len(percentChars)

	startIndex := barLength/2 - percentLen/2
	if startIndex < 0 {
		startIndex = 0
	}
	if startIndex > barLength-percentLen {
		startIndex = barLength - percentLen
	}
	endIndex := startIndex + percentLen

	for i := 0; i < barLength; i++ {
		isFilled := i < filled
		isPercentChar := i >= startIndex && i < endIndex
		percentIndex := 0
		if isPercentChar {
			percentIndex = i - startIndex
		}

		// 计算颜色
		hue := math.Mod(1.0-float64(i)/float64(barLength)+phase, 1.0)

		t := math.Mod(elapsed.Seconds()*speedFactor, 1.0)
		barLengthF64 := float64(barLength)
		iF64 := float64(i)

		mu := t * barLengthF64
		distanceRaw := math.Abs(iF64 - mu)
		distanceWrap := barLengthF64 - distanceRaw
		distance := math.Min(distanceRaw, distanceWrap)

		distanceRatio := distance / waveWidth
		attenuation := math.Exp(-distanceRatio * distanceRatio)
		brightness := progressBarBrightness[0] + progressBarBrightness[1]*attenuation
		saturation := saturationBase * (0.6 + 0.4*attenuation)
		r, g, bb := hsvToRGB(hue, saturation, brightness)

		var bgR, bgG, bgB uint8
		if isFilled {
			bgR, bgG, bgB = r, g, bb
		} else {
			bgR, bgG, bgB = unfilledBg[0], unfilledBg[1], unfilledBg[2]
		}

		if isPercentChar && percentIndex < len(percentChars) {
			c := percentChars[percentIndex]
			barStr += fmt.Sprintf("\x1b[48;2;%d;%d;%dm\x1b[1;97m%c\x1b[0m", bgR, bgG, bgB, c)
		} else {
			barStr += fmt.Sprintf("\x1b[48;2;%d;%d;%dm \x1b[0m", bgR, bgG, bgB)
		}
	}

	return barStr
}

// hsvToRGB HSV 转 RGB
func hsvToRGB(h, s, v float64) (uint8, uint8, uint8) {
	i := int(math.Floor(h * 6))
	f := h*6 - float64(i)
	p := v * (1 - s)
	q := v * (1 - f*s)
	t := v * (1 - (1-f)*s)

	var r, g, b float64
	switch i % 6 {
	case 0:
		r, g, b = v, t, p
	case 1:
		r, g, b = q, v, p
	case 2:
		r, g, b = p, v, t
	case 3:
		r, g, b = p, q, v
	case 4:
		r, g, b = t, p, v
	default:
		r, g, b = v, p, q
	}

	return uint8(r * 255), uint8(g * 255), uint8(b * 255)
}

// getTerminalWidth 获取终端宽度
func getTerminalWidth() int {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width < 20 {
		return terminalDefaultWidth
	}
	return width
}

// Grow 增加进度
func (b *Bar) Grow(num int, myStrVal string) {
	if b.inner.isCompleted() || IsProgressStopped() {
		return
	}

	b.inner.textMu.Lock()
	b.inner.text.pos += num
	b.inner.text.prefix = myStrVal
	b.inner.textMu.Unlock()
}

// Update 更新进度和消息
func (b *Bar) Update(pos int, msg string, prefix string) {
	if b.inner.isCompleted() || IsProgressStopped() {
		return
	}

	b.inner.textMu.Lock()
	b.inner.text.pos = pos
	b.inner.text.msg = msg
	b.inner.text.prefix = prefix
	b.inner.textMu.Unlock()
}

// SetPrefix 设置前缀
func (b *Bar) SetPrefix(prefix string) {
	if b.inner.isCompleted() || IsProgressStopped() {
		return
	}

	b.inner.textMu.Lock()
	b.inner.text.prefix = prefix
	b.inner.textMu.Unlock()
}

// Done 完成进度条
func (b *Bar) Done() {
	if b.inner.isCompleted() || IsProgressStopped() {
		return
	}

	b.inner.setCompleted(true)
	close(b.stopChan)

	// 等待渲染完成
	time.Sleep(refreshIntervalMs * 2 * time.Millisecond)
	os.Stdout.Sync()
}

// NewPingBar 创建延迟测速进度条
func NewPingBar(count int) *Bar {
	return NewBar(count, "", "")
}

// NewDownloadBar 创建下载测速进度条
func NewDownloadBar(count int) *Bar {
	return NewBar(count, "", "")
}
