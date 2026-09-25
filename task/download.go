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

package task

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/masgzy/CloudflareST/utils"

	"github.com/VividCortex/ewma"
)

const (
	bufferSize = 256 * 1024 // 256KB，减少高速下载时的 IO 调用次数
	// 与上游保持一致的默认测速地址（issue #1 教训）：
	// Parallels 大文件（700MB）的 HEAD 请求在 CF 边缘需要回源/查元数据，实测响应高达 ~230ms，
	// 会整体抬高 HTTPing 测量基线，导致 -tl 过滤误杀；cf.xiu2.xyz 由边缘直接应答（实测 ~6ms）。
	defaultURL                     = "https://cf.xiu2.xyz/url"
	defaultTimeout                 = 10 * time.Second
	defaultDisableDownload         = false
	defaultTestNum                 = 10
	defaultMinSpeed        float64 = 0.0
)

var (
	URL     = defaultURL
	Timeout = defaultTimeout
	Disable = defaultDisableDownload

	TestCount = defaultTestNum
	MinSpeed  = defaultMinSpeed
)

func checkDownloadDefault() {
	if URL == "" {
		URL = defaultURL
	}
	if Timeout <= 0 {
		Timeout = defaultTimeout
	}
	if TestCount <= 0 {
		TestCount = defaultTestNum
	}
	if MinSpeed <= 0.0 {
		MinSpeed = defaultMinSpeed
	}
}

// DownloadProgress 下载进度信息
type DownloadProgress struct {
	successCount int32 // 成功数量
	failCount    int32 // 失败数量
	totalCount   int32 // 总处理数量
	currentSpeed int64 // 当前速度（字节/秒）
}

func TestDownloadSpeed(ipSet utils.PingDelaySet) (speedSet utils.DownloadSpeedSet) {
	checkDownloadDefault()
	if Disable {
		return utils.DownloadSpeedSet(ipSet)
	}
	if len(ipSet) <= 0 { // IP 数组长度(IP数量) 大于 0 时才会继续下载测速
		utils.Info("延迟测速结果 IP 数量为 0，跳过下载测速。")
		return
	}
	testNum := TestCount                        // 等待下载测速的队列数量 先默认等于 下载测速数量(-dn）
	if len(ipSet) < TestCount || MinSpeed > 0 { // 如果延迟测速并过滤后的 IP 数组长度(IP数量) 小于 下载测速数量(-dn），（即 -dn 预期数量是不够的），或者指定了 下载测速下限 (-sl) 条件（这就可能要全部下载测速一遍，直到找齐预期数量或测完为止），则 等待下载测速的队列数量 修正为 IP 数量
		testNum = len(ipSet)
	}
	if testNum < TestCount { // 如果 等待下载测速的队列数量 小于 下载测速数量(-dn），（显然 -dn 预期数量是不够的），所以 下载测速数量(-dn）修正为 等待下载测速的队列数量
		TestCount = testNum
	}

	fmt.Printf("开始下载测速（下限：%.2f MB/s, 所需：%d, 队列：%d）\n", MinSpeed, TestCount, testNum)

	// 创建进度跟踪器
	progress := &DownloadProgress{}

	// 创建进度条
	bar := utils.NewDownloadBar(testNum)

	// 启动进度更新协程
	stopUpdate := make(chan struct{})
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stopUpdate:
				return
			case <-ticker.C:
				// 检查全局停止标志
				if atomic.LoadInt32(&GlobalEarlyStop) == 1 {
					return
				}
				success := atomic.LoadInt32(&progress.successCount)
				fail := atomic.LoadInt32(&progress.failCount)
				done := int(atomic.LoadInt32(&progress.totalCount))
				speed := atomic.LoadInt64(&progress.currentSpeed)
				speedMB := float64(speed) / 1024 / 1024

				// 更新进度条：显示 成功|失败 进度条 速率
				var msg string
				var pre string
				if utils.SupportsColor() {
					msg = fmt.Sprintf("\x1b[33m%d|%d\x1b[0m", success, fail)
					pre = fmt.Sprintf("\x1b[92m%.2f\x1b[0m MB/s", speedMB)
				} else {
					msg = fmt.Sprintf("%d|%d", success, fail)
					pre = fmt.Sprintf("%.2f MB/s", speedMB)
				}
				bar.Update(done, msg, pre)
			}
		}
	}()

	for i := 0; i < testNum; i++ {
		// 检查是否已经凑够数量或全局超时
		if int(atomic.LoadInt32(&progress.successCount)) >= TestCount || atomic.LoadInt32(&GlobalEarlyStop) == 1 {
			break
		}

		speed, colo := downloadHandlerWithProgress(ipSet[i].IP, progress)
		ipSet[i].DownloadSpeed = speed
		if ipSet[i].Colo == "" { // 只有当 Colo 是空的时候，才写入，否则代表之前是 httping 测速并获取过了
			ipSet[i].Colo = colo
		}

		// 在每个 IP 下载测速后，以 [下载速度下限] 条件过滤结果
		if speed >= MinSpeed*1024*1024 {
			atomic.AddInt32(&progress.successCount, 1)
			speedSet = append(speedSet, ipSet[i]) // 高于下载速度下限时，添加到新数组中
		} else {
			atomic.AddInt32(&progress.failCount, 1)
		}

		// 更新总处理数量（用于进度条显示）
		atomic.AddInt32(&progress.totalCount, 1)
	}

	// 停止进度更新协程
	close(stopUpdate)

	// 检查是否因为全局停止而退出
	if atomic.LoadInt32(&GlobalEarlyStop) == 1 {
		bar.Done()
		if len(speedSet) == 0 && len(ipSet) > 0 {
			// 全局超时等场景下下载测速被整体跳过：
			// 直接沿用延迟测速结果（保持延迟排序，与 -dd 行为一致），避免已测得的结果被丢弃
			speedSet = utils.DownloadSpeedSet(ipSet)
			return
		}
		// 按速度排序
		speedSet.Sort()
		return
	}

	// 最终更新一次进度条
	success := atomic.LoadInt32(&progress.successCount)
	fail := atomic.LoadInt32(&progress.failCount)
	done := int(atomic.LoadInt32(&progress.totalCount))
	var msg, prefix string
	if utils.SupportsColor() {
		msg = fmt.Sprintf("\x1b[33m%d|%d\x1b[0m", success, fail)
		prefix = "完成"
	} else {
		msg = fmt.Sprintf("%d|%d", success, fail)
		prefix = "完成"
	}
	bar.Update(done, msg, prefix)

	bar.Done()

	if MinSpeed == 0.00 { // 如果没有指定下载速度下限，则直接返回所有测速数据
		speedSet = utils.DownloadSpeedSet(ipSet)
	} else if utils.Debug && len(speedSet) == 0 { // 如果指定了下载速度下限，且是调试模式下，且没有找到任何一个满足条件的 IP 时，返回所有测速数据，供用户查看当前的测速结果，以便适当调低预期测速条件
		utils.Debugf("没有满足 下载速度下限 条件的 IP，忽略条件返回所有测速数据（方便下次测速时调整条件）。")
		speedSet = utils.DownloadSpeedSet(ipSet)
	}
	// 按速度排序
	speedSet.Sort()
	return
}

func getDialContext(ip *net.IPAddr) func(ctx context.Context, network, address string) (net.Conn, error) {
	var fakeSourceAddr string
	port := GetPortForIP(ip.IP)
	if isIPv4(ip.String()) {
		fakeSourceAddr = fmt.Sprintf("%s:%d", ip.String(), port)
	} else {
		fakeSourceAddr = fmt.Sprintf("[%s]:%d", ip.String(), port)
	}
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		dialer := &net.Dialer{}
		// 如果指定了绑定接口或本地 IP
		if BindIntf != "" {
			// 检查是否是 IP 地址格式
			if bindIP := net.ParseIP(BindIntf); bindIP != nil {
				// 是 IP 地址，设置 LocalAddr（IPv4/IPv6 均适用）
				dialer.LocalAddr = &net.TCPAddr{IP: bindIP}
			} else {
				// 不是 IP 地址，认为是接口名，通过 Control 函数绑定
				dialer.Control = getBindInterfaceControl(BindIntf)
			}
		}
		// 合并 SO_LINGER(0) 与已有的 Control（接口绑定），跳过 TIME_WAIT
		// （IP_BIND_ADDRESS_NO_PORT 仅在显式 bind 源 IP 时生效，其余路径为 no-op）
		dialer.Control = chainControl(setLingerControl(), setBindAddressNoPortControl(), dialer.Control)
		return dialer.DialContext(ctx, network, fakeSourceAddr)
	}
}

// bufferPool 复用下载缓冲区，减少 GC 压力
var bufferPool = sync.Pool{
	New: func() interface{} {
		b := make([]byte, bufferSize)
		return &b
	},
}

func getBuffer() *[]byte {
	return bufferPool.Get().(*[]byte)
}

func putBuffer(b *[]byte) {
	bufferPool.Put(b)
}

// 统一的请求报错调试输出
func printDownloadDebugInfo(ip *net.IPAddr, err error, statusCode int, url, lastRedirectURL string, response *http.Response) {
	finalURL := url // 默认的最终 URL，这样当 response 为空时也能输出
	if lastRedirectURL != "" {
		finalURL = lastRedirectURL // 如果 lastRedirectURL 不是空，说明重定向过，优先输出最后一次要重定向至的目标
	} else if response != nil && response.Request != nil && response.Request.URL != nil {
		finalURL = response.Request.URL.String() // 如果 response 不为 nil，且 Request 和 URL 都不为 nil，则获取最后一次成功的响应地址
	}
	if url != finalURL { // 如果 URL 和最终地址不一致，说明有重定向，是该重定向后的地址引起的错误
		if statusCode > 0 { // 如果状态码大于 0，说明是后续 HTTP 状态码引起的错误
			utils.Debugf("IP: %s, 下载测速终止，HTTP 状态码: %d, 下载测速地址: %s, 出错的重定向后地址: %s", ip.String(), statusCode, url, finalURL)
		} else {
			utils.Debugf("IP: %s, 下载测速失败，错误信息: %v, 下载测速地址: %s, 出错的重定向后地址: %s", ip.String(), err, url, finalURL)
		}
	} else { // 如果 URL 和最终地址一致，说明没有重定向
		if statusCode > 0 { // 如果状态码大于 0，说明是后续 HTTP 状态码引起的错误
			utils.Debugf("IP: %s, 下载测速终止，HTTP 状态码: %d, 下载测速地址: %s", ip.String(), statusCode, url)
		} else {
			utils.Debugf("IP: %s, 下载测速失败，错误信息: %v, 下载测速地址: %s", ip.String(), err, url)
		}
	}
}

// downloadHandlerWithProgress 带进度更新的下载处理
func downloadHandlerWithProgress(ip *net.IPAddr, progress *DownloadProgress) (float64, string) {
	var lastRedirectURL string // 用于记录最后一次重定向目标，以便在访问错误时输出
	client := &http.Client{
		Transport: &http.Transport{
			DialContext:       getDialContext(ip),
			DisableKeepAlives: true,  // 测速无需连接池，每次含完整握手
			ForceAttemptHTTP2: false, // 测速场景禁用 HTTP/2 多路复用
		},
		Timeout: Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			lastRedirectURL = req.URL.String() // 记录每次重定向的目标，以便在访问错误时输出
			if len(via) > 10 {                 // 限制最多重定向 10 次
				if utils.Debug { // 调试模式下，输出更多信息
					utils.Debugf("IP: %s, 下载测速地址重定向次数过多，终止测速，下载测速地址: %s", ip.String(), req.URL.String())
				}
				return http.ErrUseLastResponse
			}
			if req.Header.Get("Referer") == defaultURL { // 当使用默认下载测速地址时，重定向不携带 Referer
				req.Header.Del("Referer")
			}
			return nil
		},
	}
	defer client.CloseIdleConnections()
	req, err := http.NewRequest("GET", URL, nil)
	if err != nil {
		if utils.Debug { // 调试模式下，输出更多信息
			utils.Debugf("IP: %s, 下载测速请求创建失败，错误信息: %v, 下载测速地址: %s", ip.String(), err, URL)
		}
		atomic.StoreInt64(&progress.currentSpeed, 0)
		return 0.0, ""
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_12_6) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/98.0.4758.80 Safari/537.36")

	response, err := client.Do(req)
	if err != nil {
		if utils.Debug { // 调试模式下，输出更多信息
			printDownloadDebugInfo(ip, err, 0, URL, lastRedirectURL, response)
		}
		atomic.StoreInt64(&progress.currentSpeed, 0)
		return 0.0, ""
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		if utils.Debug { // 调试模式下，输出更多信息
			printDownloadDebugInfo(ip, nil, response.StatusCode, URL, lastRedirectURL, response)
		}
		atomic.StoreInt64(&progress.currentSpeed, 0)
		return 0.0, ""
	}

	// 通过头部参数获取地区码
	colo := getHeaderColo(response.Header)

	timeStart := time.Now()           // 开始时间（当前）
	timeEnd := timeStart.Add(Timeout) // 加上下载测速时间得到的结束时间

	contentLength := response.ContentLength // 文件大小
	bufferPtr := getBuffer()
	defer putBuffer(bufferPtr)
	buffer := *bufferPtr

	var contentRead int64 = 0

	// EWMA 用于实时速度显示：平滑「全局平均速度」的变化趋势，使实时显示逐渐收敛到最终结果
	// 参数 10 表示约 10 个样本（1 秒）的衰减窗口，比默认的 30 秒窗口更适合短时测速
	e := ewma.NewMovingAverage(10)
	lastSampleTime := timeStart
	const sampleInterval = 100 * time.Millisecond // 采样间隔

	// 循环计算，如果文件下载完了（两者相等），则退出循环（终止测速）
	for contentLength != contentRead {
		// 检查全局停止标志
		if atomic.LoadInt32(&GlobalEarlyStop) == 1 {
			break
		}
		currentTime := time.Now()
		// 如果超出下载测速时间，则退出循环（终止测速）
		if currentTime.After(timeEnd) {
			break
		}
		// 定期采样：计算全局平均速度（总下载量 / 已用时间）并用 EWMA 平滑
		// 与 speedtest-go 的做法一致：EWMA 平滑的是全局平均速度，而非瞬时速度
		// 这样实时显示会逐渐收敛到最终结果，减少两者之间的差异
		if currentTime.Sub(lastSampleTime) >= sampleInterval {
			elapsed := currentTime.Sub(timeStart).Seconds()
			if elapsed > 0 {
				globalAvg := float64(contentRead) / elapsed
				e.Add(globalAvg)
				atomic.StoreInt64(&progress.currentSpeed, int64(e.Value()))
			}
			lastSampleTime = currentTime
		}
		bufferRead, err := response.Body.Read(buffer)
		contentRead += int64(bufferRead)
		if err != nil {
			// EOF 且已读满 contentLength：循环条件自然退出，break 等价；
			// EOF 但未读满：服务器提前断开（短读），继续 Read 只会永远返回 EOF 空转烧 CPU 直到超时；
			// 其他错误（如超时）：直接终止。
			break
		}
	}

	// 最终速度：总下载量 / 实际耗时（最准确，与 speedtest-go 的 GetAvgDownloadRate 一致）
	// 不使用 EWMA，因为 EWMA 对近期数据加权更大，不代表整个测试期间的真实平均速度
	actualElapsed := time.Since(timeStart).Seconds()
	var finalSpeed float64
	if actualElapsed > 0 && contentRead > 0 {
		finalSpeed = float64(contentRead) / actualElapsed
	}
	atomic.StoreInt64(&progress.currentSpeed, int64(finalSpeed))

	return finalSpeed, colo
}
