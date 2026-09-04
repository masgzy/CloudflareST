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
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/masgzy/CloudflareST/utils"
)

const (
	tcpConnectTimeout = time.Second * 1
	defaultRoutines   = 200
	defaultPort       = 443
	defaultPingTimes  = 4
)

var (
	Routines         = defaultRoutines
	TCPPort      int = defaultPort
	PingTimes    int = defaultPingTimes
	TargetNum    int = 0                // 延迟测速可用数量目标，0表示不限制
	PingInterval     = time.Duration(0) // 每次 ping 之间的间隔，默认 0 不间隔
)

// weightedSemaphore 是一个轻量级加权信号量实现，避免引入 golang.org/x/sync 依赖
// 保持与 Go 1.20 的兼容性（old 版本构建）
type weightedSemaphore struct {
	ch chan struct{}
}

func newWeightedSemaphore(n int64) *weightedSemaphore {
	return &weightedSemaphore{ch: make(chan struct{}, n)}
}

func (s *weightedSemaphore) Acquire(ctx context.Context, n int64) error {
	select {
	case s.ch <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *weightedSemaphore) Release(n int64) {
	for i := int64(0); i < n; i++ { // 按请求量释放，避免调用 Release(2) 却只释放 1 个配额
		<-s.ch
	}
}

type Ping struct {
	wg          *sync.WaitGroup
	m           *sync.Mutex
	ips         []*net.IPAddr
	csv         utils.PingDelaySet
	sem         *weightedSemaphore
	bar         *utils.Bar
	earlyStop   int32 // 原子标志：是否提前停止
	totalCount  int32 // 原子计数器：已处理的IP总数
	usableCount int32 // 原子计数器：用于显示的可用数量
}

func checkPingDefault() {
	if Routines <= 0 {
		Routines = defaultRoutines
	}
	if TCPPort <= 0 || TCPPort > 65535 {
		TCPPort = defaultPort
	}
	if PingTimes <= 0 {
		PingTimes = defaultPingTimes
	}
}

func NewPing() *Ping {
	checkPingDefault()
	ips := loadIPRanges()
	return &Ping{
		wg:          &sync.WaitGroup{},
		m:           &sync.Mutex{},
		ips:         ips,
		csv:         make(utils.PingDelaySet, 0),
		sem:         newWeightedSemaphore(int64(Routines)),
		bar:         utils.NewPingBar(len(ips)),
		earlyStop:   0,
		totalCount:  0,
		usableCount: 0,
	}
}

func (p *Ping) Run() utils.PingDelaySet {
	if len(p.ips) == 0 {
		return p.csv
	}
	if Httping {
		fmt.Printf("开始延迟测速（模式：HTTP, 端口：%d, 范围：%v ~ %v ms, 丢包：%.2f）\n", TCPPort, utils.InputMinDelay.Milliseconds(), utils.InputMaxDelay.Milliseconds(), utils.InputMaxLossRate)
	} else {
		fmt.Printf("开始延迟测速（模式：TCP, 端口：%d, 范围：%v ~ %v ms, 丢包：%.2f）\n", TCPPort, utils.InputMinDelay.Milliseconds(), utils.InputMaxDelay.Milliseconds(), utils.InputMaxLossRate)
	}
	for _, ip := range p.ips {
		// 检查是否需要提前停止（局部或全局）
		if atomic.LoadInt32(&p.earlyStop) == 1 || atomic.LoadInt32(&GlobalEarlyStop) == 1 {
			break
		}
		p.wg.Add(1)
		// context.Background 永不取消，Acquire 只会在拿到配额后返回 nil，错误可安全忽略
		_ = p.sem.Acquire(context.Background(), 1)
		go p.start(ip)
	}
	p.wg.Wait()
	p.bar.Done()
	sort.Sort(p.csv)
	return p.csv
}

func (p *Ping) start(ip *net.IPAddr) {
	defer p.wg.Done()
	defer p.sem.Release(1)

	// 检查是否需要提前停止（局部或全局）
	if atomic.LoadInt32(&p.earlyStop) == 1 || atomic.LoadInt32(&GlobalEarlyStop) == 1 {
		return
	}

	p.tcpingHandler(ip)
}

// bool connectionSucceed float32 time
func (p *Ping) tcping(ip *net.IPAddr) (bool, time.Duration) {
	startTime := time.Now()
	port := GetPortForIP(ip.IP)
	var fullAddress string
	if isIPv4(ip.String()) {
		fullAddress = fmt.Sprintf("%s:%d", ip.String(), port)
	} else {
		fullAddress = fmt.Sprintf("[%s]:%d", ip.String(), port)
	}

	dialer := &net.Dialer{Timeout: tcpConnectTimeout}
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
	dialer.Control = chainControl(setLingerControl(), dialer.Control)

	conn, err := dialer.Dial("tcp", fullAddress)
	if err != nil {
		return false, 0
	}
	_ = conn.Close() // SO_LINGER(0) 使 close() 发 RST，无 TIME_WAIT
	duration := time.Since(startTime)
	return true, duration
}

// pingReceived pingTotalTime
func (p *Ping) checkConnection(ip *net.IPAddr) (recv int, totalDelay time.Duration, colo string) {
	if Httping {
		recv, totalDelay, colo = p.httping(ip)
		return
	}
	colo = "" // TCPing 不获取 colo
	for i := 0; i < PingTimes; i++ {
		// 在每次 ping 前检查是否需要提前停止（局部或全局）
		if atomic.LoadInt32(&p.earlyStop) == 1 || atomic.LoadInt32(&GlobalEarlyStop) == 1 {
			return
		}
		if ok, delay := p.tcping(ip); ok {
			recv++
			totalDelay += delay
			// 借鉴 GY-rust：只有成功才 sleep，失败不 sleep
			if PingInterval > 0 && i < PingTimes-1 {
				time.Sleep(PingInterval)
			}
		}
	}
	return
}

// tryAppendIPData 尝试添加IP数据，返回是否成功
func (p *Ping) tryAppendIPData(data *utils.PingData) bool {
	p.m.Lock()
	defer p.m.Unlock()

	// 检查是否已经达到目标数量
	if TargetNum > 0 && len(p.csv) >= TargetNum {
		return false
	}

	p.csv = append(p.csv, utils.CloudflareIPData{
		PingData: data,
	})

	currentCount := len(p.csv)
	// 更新可用计数（与实际数据同步）
	atomic.StoreInt32(&p.usableCount, int32(currentCount))

	// 当达到目标数量时，设置停止标志
	if TargetNum > 0 && currentCount >= TargetNum {
		atomic.StoreInt32(&p.earlyStop, 1)
	}

	return true
}

// handle tcping
func (p *Ping) tcpingHandler(ip *net.IPAddr) {
	// 在开始测试前再次检查（局部或全局）
	if atomic.LoadInt32(&p.earlyStop) == 1 || atomic.LoadInt32(&GlobalEarlyStop) == 1 {
		return
	}

	recv, totalDlay, colo := p.checkConnection(ip)

	// 增加已处理计数
	done := int(atomic.AddInt32(&p.totalCount, 1))

	// 测试完成后再次检查是否需要停止（局部或全局）
	if atomic.LoadInt32(&p.earlyStop) == 1 || atomic.LoadInt32(&GlobalEarlyStop) == 1 {
		// 更新进度条
		usable := atomic.LoadInt32(&p.usableCount)
		p.bar.Update(done, fmt.Sprintf("%d/%d", done, len(p.ips)), fmt.Sprintf("\x1b[37m可用:\x1b[0m \x1b[92m%d\x1b[0m", usable))
		return
	}

	if recv != 0 {
		avgDelay := totalDlay / time.Duration(recv)
		// 只有平均延迟在上限内才尝试添加
		if avgDelay <= utils.InputMaxDelay {
			data := &utils.PingData{
				IP:           ip,
				Sended:       PingTimes,
				Received:     recv,
				Delay:        avgDelay,
				Colo:         colo,
				Port:         GetPortForIP(ip.IP),
				PortFromUser: IPPortFromUser[ip.String()],
			}
			// 尝试添加数据
			p.tryAppendIPData(data)
		}
	}

	// 更新进度条：显示已完成的和可用数量
	usable := atomic.LoadInt32(&p.usableCount)
	p.bar.Update(done, fmt.Sprintf("%d/%d", done, len(p.ips)), fmt.Sprintf("\x1b[37m可用:\x1b[0m \x1b[92m%d\x1b[0m", usable))
}
