package task

import (
	"fmt"
	"net"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/XIU2/CloudflareSpeedTest/utils"
)

const (
	tcpConnectTimeout = time.Second * 1
	maxRoutine        = 1000
	defaultRoutines   = 200
	defaultPort       = 443
	defaultPingTimes  = 4
)

var (
	Routines  = defaultRoutines
	TCPPort   int = defaultPort
	PingTimes int = defaultPingTimes
	TargetNum int = 0 // 延迟测速可用数量目标，0表示不限制
)

type Ping struct {
	wg          *sync.WaitGroup
	m           *sync.Mutex
	ips         []*net.IPAddr
	csv         utils.PingDelaySet
	control     chan bool
	bar         *utils.Bar
	earlyStop   int32 // 原子标志：是否提前停止
	totalCount  int32 // 原子计数器：已处理的IP总数
	usableCount int32 // 原子计数器：用于显示的可用数量
}

func checkPingDefault() {
	if Routines <= 0 {
		Routines = defaultRoutines
	}
	if TCPPort <= 0 || TCPPort >= 65535 {
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
		control:     make(chan bool, Routines),
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
		// 检查是否需要提前停止
		if atomic.LoadInt32(&p.earlyStop) == 1 {
			break
		}
		p.wg.Add(1)
		p.control <- false
		go p.start(ip)
	}
	p.wg.Wait()
	p.bar.Done()
	sort.Sort(p.csv)
	return p.csv
}

func (p *Ping) start(ip *net.IPAddr) {
	defer p.wg.Done()
	defer func() { <-p.control }()

	// 检查是否需要提前停止
	if atomic.LoadInt32(&p.earlyStop) == 1 {
		return
	}

	p.tcpingHandler(ip)
}

// bool connectionSucceed float32 time
func (p *Ping) tcping(ip *net.IPAddr) (bool, time.Duration) {
	startTime := time.Now()
	var fullAddress string
	if isIPv4(ip.String()) {
		fullAddress = fmt.Sprintf("%s:%d", ip.String(), TCPPort)
	} else {
		fullAddress = fmt.Sprintf("[%s]:%d", ip.String(), TCPPort)
	}
	conn, err := net.DialTimeout("tcp", fullAddress, tcpConnectTimeout)
	if err != nil {
		return false, 0
	}
	defer conn.Close()
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
		// 在每次 ping 前检查是否需要提前停止
		if atomic.LoadInt32(&p.earlyStop) == 1 {
			return
		}
		if ok, delay := p.tcping(ip); ok {
			recv++
			totalDelay += delay
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
	// 在开始测试前再次检查
	if atomic.LoadInt32(&p.earlyStop) == 1 {
		return
	}

	recv, totalDlay, colo := p.checkConnection(ip)

	// 增加已处理计数
	done := int(atomic.AddInt32(&p.totalCount, 1))

	// 测试完成后再次检查是否需要停止
	if atomic.LoadInt32(&p.earlyStop) == 1 {
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
				IP:       ip,
				Sended:   PingTimes,
				Received: recv,
				Delay:    avgDelay,
				Colo:     colo,
			}
			// 尝试添加数据
			p.tryAppendIPData(data)
		}
	}

	// 更新进度条：显示已完成的和可用数量
	usable := atomic.LoadInt32(&p.usableCount)
	p.bar.Update(done, fmt.Sprintf("%d/%d", done, len(p.ips)), fmt.Sprintf("\x1b[37m可用:\x1b[0m \x1b[92m%d\x1b[0m", usable))
}
