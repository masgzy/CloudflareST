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
	"encoding/csv"
	"fmt"
	"log"
	"math"
	"net"
	"os"
	"sort"
	"strconv"
	"time"
)

const (
	defaultOutput         = "result.csv"
	maxDelay              = 9999 * time.Millisecond
	minDelay              = 0 * time.Millisecond
	maxLossRate   float32 = 1.0
)

var (
	InputMaxDelay    = maxDelay
	InputMinDelay    = minDelay
	InputMaxLossRate = maxLossRate
	Output           = defaultOutput
	PrintNum         = 10
	Debug            = false // 是否开启调试模式
	ShowPort         = false // 是否在结果中显示端口号（-sp 控制）
	UseZScore        = false // 是否启用综合排序（-zscore 控制）
)

// 是否打印测试结果
func NoPrintResult() bool {
	return PrintNum == 0
}

// 是否输出到文件
func noOutput() bool {
	return Output == "" || Output == " "
}

type PingData struct {
	IP       *net.IPAddr
	Sended   int
	Received int
	Delay    time.Duration
	Colo     string
	// Port 单个 IP 的自定义测速端口，0 表示使用全局默认（utils.TCPPort）
	Port int
	// PortFromUser 标记该 IP 的端口是否由用户在 -ip/-f 中显式指定
	PortFromUser bool
}

// 拼接 IP 与端口：显式 -sp 或用户在 -ip/-f 中指定了端口时显示
func (cf *PingData) formatIPWithPort() string {
	if cf.Port > 0 && (ShowPort || cf.PortFromUser) {
		return net.JoinHostPort(cf.IP.String(), strconv.Itoa(cf.Port))
	}
	return cf.IP.String()
}

type CloudflareIPData struct {
	*PingData
	lossRate      float32
	DownloadSpeed float64
}

// 计算丢包率
func (cf *CloudflareIPData) getLossRate() float32 {
	if cf.lossRate == 0 {
		pingLost := cf.Sended - cf.Received
		cf.lossRate = float32(pingLost) / float32(cf.Sended)
	}
	return cf.lossRate
}

// 拼接 IP 与端口：没有端口时只返回 IP
// （保留此包装供 toString 使用，逻辑已移至 PingData.formatIPWithPort）
func (cf *CloudflareIPData) toString() []string {
	result := make([]string, 7)
	result[0] = cf.PingData.formatIPWithPort()
	result[1] = strconv.Itoa(cf.Sended)
	result[2] = strconv.Itoa(cf.Received)
	result[3] = strconv.FormatFloat(float64(cf.getLossRate()), 'f', 2, 32)
	result[4] = strconv.FormatFloat(cf.Delay.Seconds()*1000, 'f', 2, 32)
	result[5] = strconv.FormatFloat(cf.DownloadSpeed/1024/1024, 'f', 2, 32)
	// 如果 Colo 为空，则使用 "N/A" 表示
	if cf.Colo == "" {
		result[6] = "N/A"
	} else {
		result[6] = cf.Colo
	}
	return result
}

func ExportCsv(data []CloudflareIPData) {
	if noOutput() || len(data) == 0 {
		return
	}
	fp, err := os.Create(Output)
	if err != nil {
		log.Fatalf("创建文件[%s]失败：%v", Output, err)
	}
	defer fp.Close()
	// 写入 UTF-8 BOM，确保 Excel 直接打开时中文表头不乱码
	if _, err := fp.WriteString("\xEF\xBB\xBF"); err != nil {
		log.Fatalf("写入文件[%s]失败：%v", Output, err)
	}
	w := csv.NewWriter(fp) //创建一个新的写入文件流
	_ = w.Write([]string{"IP 地址", "已发送", "已接收", "丢包率", "平均延迟", "下载速度(MB/s)", "地区码"})
	_ = w.WriteAll(convertToString(data))
	w.Flush()
}

func convertToString(data []CloudflareIPData) [][]string {
	result := make([][]string, 0)
	for _, v := range data {
		result = append(result, v.toString())
	}
	return result
}

// 延迟丢包排序
type PingDelaySet []CloudflareIPData

// 延迟条件过滤
func (s PingDelaySet) FilterDelay() (data PingDelaySet) {
	if InputMaxDelay > maxDelay || InputMinDelay < minDelay { // 当输入的延迟条件不在默认范围内时，不进行过滤
		return s
	}
	if InputMaxDelay == maxDelay && InputMinDelay == minDelay { // 当输入的延迟条件为默认值时，不进行过滤
		return s
	}
	for _, v := range s {
		// 注意：数组主排序键是丢包率（其次才是延迟），延迟并非单调递增，
		// 因此这里只能 continue 跳过，绝不能 break——否则会把排在后面、
		// 丢包率稍高但延迟满足条件的 IP 误杀（尤其默认 -tlr 1.0 不过滤丢包时）
		if v.Delay > InputMaxDelay { // 平均延迟上限
			continue
		}
		if v.Delay < InputMinDelay { // 平均延迟下限
			continue
		}
		data = append(data, v) // 延迟满足条件时，添加到新数组中
	}
	return
}

// 丢包条件过滤
func (s PingDelaySet) FilterLossRate() (data PingDelaySet) {
	if InputMaxLossRate >= maxLossRate { // 当输入的丢包条件为默认值时，不进行过滤
		return s
	}
	for _, v := range s {
		if v.getLossRate() > InputMaxLossRate { // 丢包几率上限
			break
		}
		data = append(data, v) // 丢包率满足条件时，添加到新数组中
	}
	return
}

func (s PingDelaySet) Len() int {
	return len(s)
}
func (s PingDelaySet) Less(i, j int) bool {
	iRate, jRate := s[i].getLossRate(), s[j].getLossRate()
	if iRate != jRate {
		return iRate < jRate
	}
	return s[i].Delay < s[j].Delay
}
func (s PingDelaySet) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// 下载速度排序
type DownloadSpeedSet []CloudflareIPData

func (s DownloadSpeedSet) Len() int {
	return len(s)
}
func (s DownloadSpeedSet) Less(i, j int) bool {
	return s[i].DownloadSpeed > s[j].DownloadSpeed
}
func (s DownloadSpeedSet) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// Sort 根据是否启用 -zscore 选择排序方式
func (s DownloadSpeedSet) Sort() {
	if !UseZScore || len(s) < 2 {
		sort.Sort(s) // 默认纯速度排序
		return
	}
	// z-score 归一化排序（参照 GY-rust common.rs）
	n := float64(len(s))
	var sumDelay, sumSpeed, sumLoss float64
	for _, v := range s {
		sumDelay += float64(v.Delay.Milliseconds())
		sumSpeed += v.DownloadSpeed
		sumLoss += float64(v.getLossRate())
	}
	meanDelay := sumDelay / n
	meanSpeed := sumSpeed / n
	meanLoss := sumLoss / n

	var varDelay, varSpeed, varLoss float64
	for _, v := range s {
		varDelay += math.Pow(float64(v.Delay.Milliseconds())-meanDelay, 2)
		varSpeed += math.Pow(v.DownloadSpeed-meanSpeed, 2)
		varLoss += math.Pow(float64(v.getLossRate())-meanLoss, 2)
	}
	stdDelay := math.Sqrt(varDelay / n)
	stdSpeed := math.Sqrt(varSpeed / n)
	stdLoss := math.Sqrt(varLoss / n)

	// 权重：速度、延迟、丢包率均等权重（与 GY-rust 一致）
	const wDelay, wSpeed, wLoss = 1.0, 1.0, 1.0
	score := func(v CloudflareIPData) float64 {
		zDelay := safeDiv(float64(v.Delay.Milliseconds())-meanDelay, stdDelay)
		zSpeed := safeDiv(v.DownloadSpeed-meanSpeed, stdSpeed)
		zLoss := safeDiv(float64(v.getLossRate())-meanLoss, stdLoss)
		return wSpeed*zSpeed - wDelay*zDelay - wLoss*zLoss
	}
	sort.Slice(s, func(i, j int) bool {
		return score(s[i]) > score(s[j])
	})
}

// safeDiv 安全除法，避免除以 0
func safeDiv(a, b float64) float64 {
	if math.Abs(b) < 1e-9 {
		return 0
	}
	return a / b
}

func (s DownloadSpeedSet) Print() {
	if NoPrintResult() {
		return
	}
	if len(s) <= 0 { // IP数组长度(IP数量) 大于 0 时继续
		fmt.Println("\n[信息] 完整测速结果 IP 数量为 0，跳过输出结果。")
		return
	}
	dateString := convertToString(s) // 转为多维数组 [][]String
	if len(dateString) < PrintNum {  // 如果IP数组长度(IP数量) 小于  打印次数，则次数改为IP数量
		PrintNum = len(dateString)
	}
	headFormat := "%-16s%-5s%-5s%-5s%-6s%-12s%-5s\n"
	dataFormat := "%-18s%-8s%-8s%-8s%-10s%-16s%-8s\n"
	ipv6Format := "%-40s%-5s%-5s%-5s%-6s%-12s%-5s\n"
	ipv6DataFormat := "%-42s%-8s%-8s%-8s%-10s%-16s%-8s\n"
	for i := 0; i < PrintNum; i++ { // 如果要输出的 IP 中包含 IPv6 或端口(>15字符)，则调整列宽
		if len(dateString[i][0]) > 15 {
			headFormat = ipv6Format
			dataFormat = ipv6DataFormat
			break
		}
	}
	Cyan.Printf(headFormat, "IP 地址", "已发送", "已接收", "丢包率", "平均延迟", "下载速度(MB/s)", "地区码")
	for i := 0; i < PrintNum; i++ {
		fmt.Printf(dataFormat, dateString[i][0], dateString[i][1], dateString[i][2], dateString[i][3], dateString[i][4], dateString[i][5], dateString[i][6])
	}
	if !noOutput() {
		fmt.Printf("\n完整测速结果已写入 %v 文件，可使用记事本/表格软件查看。\n", Output)
	}
}
