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
	"bufio"
	crand "crypto/rand"
	"encoding/binary"
	"fmt"
	"log"
	"math/big"
	"math/rand"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

const defaultInputFile = "ip.txt"

var (
	// TestAll test all ip
	TestAll = false
	// IPFile is the filename of IP Rangs
	IPFile = defaultInputFile
	IPText string
	// BindIntf 绑定到指定的网络接口或本地 IP 进行测速
	BindIntf string
	// ProgramTimeout 程序超时时间（秒）
	ProgramTimeout int
	// GlobalEarlyStop 全局停止标志，用于超时退出
	GlobalEarlyStop int32
)

// ValidateBindIntf 验证 BindIntf 参数是否有效
// 应在程序启动时调用，如果验证失败则直接退出程序
func ValidateBindIntf() {
	if BindIntf == "" {
		return // 空参数有效，表示不绑定接口
	}
	if err := ValidateBindInterface(BindIntf); err != nil {
		log.Fatal(err)
	}
}

func InitRandSeed() {
	// Go 1.20+ 已自动播种全局 rand；Go 1.24 起 Seed 正式变为 no-op，
	// 但 GODEBUG 兼容行为跟随 go.mod 的 go 指令（本项目为 go 1.18），
	// 因此即使使用 Go 1.22~1.24 工具链编译，Seed 仍保持有效，不会影响 _old 老平台构建。
	rand.Seed(time.Now().UnixNano())
}

func isIPv4(ip string) bool {
	return strings.Contains(ip, ".")
}

func randIPEndWith(num byte) byte {
	if num == 0 { // 对于 /32 这种单独的 IP
		return byte(0)
	}
	return byte(rand.Intn(int(num)))
}

var (
	// IPPortMap 全局 IP 到自定义端口映射（0 表示使用全局 TCPPort 默认值）
	IPPortMap = make(map[string]int)
	// IPPortFromUser 标记哪些 IP 的端口是由用户在 -ip/-f 中显式指定的
	IPPortFromUser = make(map[string]bool)
)

func GetPortForIP(ip net.IP) int {
	if port, ok := IPPortMap[ip.String()]; ok && port > 0 {
		return port
	}
	return TCPPort
}

type IPRanges struct {
	ips         []*net.IPAddr
	mask        string
	firstIP     net.IP
	ipNet       *net.IPNet
	currentPort int // 当前 IP/CIDR 区块的端口（解析时设置，appendIP 时使用）
}

func newIPRanges() *IPRanges {
	return &IPRanges{
		ips: make([]*net.IPAddr, 0),
	}
}

// 提取端口号，返回剥离端口后的 IP 字符串和端口号（0 表示使用全局默认）
// 支持格式：1.1.1.1:443、1.1.1.0/24:443、[::1]:443、[2606:4700::/32]:443，
// 以及无端口形式：1.1.1.1、1.1.1.0/24、::1、[::1]、[2606:4700::/32]、2606:4700::/32
// 注意：返回的 cleanIP 一律不含方括号（net.ParseCIDR 不接受方括号形式）
func extractPort(ip string) (cleanIP string, port int) {
	// 带端口形式交给标准库处理：SplitHostPort 能正确处理方括号 IPv6（"[::1]:80"）
	// 以及 CIDR 中含 "/" 的写法（"1.1.1.0/24:443"、"[2606:4700::/32]:443"）
	if host, portStr, err := net.SplitHostPort(ip); err == nil {
		if p, perr := strconv.Atoi(portStr); perr == nil && p > 0 && p < 65536 {
			return host, p // host 已由 SplitHostPort 去掉方括号
		}
		// 端口段非法（如 "1.1.1.1:abc"），整体交回给 ParseCIDR 报错
		return ip, 0
	}

	// 无端口形式：仅需去掉 IPv6 方括号（[::1] → ::1），其余原样返回
	cleanIP = strings.TrimSuffix(strings.TrimPrefix(ip, "["), "]")
	return cleanIP, 0
}

// 如果是单独 IP 则加上子网掩码，反之则获取子网掩码(r.mask)
func (r *IPRanges) fixIP(ip string) (cleanIP string, port int) {
	cleanIP, port = extractPort(ip)

	// 如果不含有 '/' 则代表不是 IP 段，而是一个单独的 IP，因此需要加上 /32 /128 子网掩码
	if i := strings.IndexByte(cleanIP, '/'); i < 0 {
		if isIPv4(cleanIP) {
			r.mask = "/32"
		} else {
			r.mask = "/128"
		}
		cleanIP += r.mask
	} else {
		r.mask = cleanIP[i:]
	}
	return
}

// 解析 IP 段，获得 IP、IP 范围、子网掩码，以及端口
func (r *IPRanges) parseCIDR(ip string) {
	var err error
	cleanIP, port := r.fixIP(ip)
	if r.firstIP, r.ipNet, err = net.ParseCIDR(cleanIP); err != nil {
		log.Fatalln("ParseCIDR err", ip, err)
	}
	// 存储当前 CIDR 区块对应的端口（后续 appendIP 时使用）
	r.currentPort = port
}

func (r *IPRanges) appendIPv4(d byte) {
	r.appendIP(net.IPv4(r.firstIP[12], r.firstIP[13], r.firstIP[14], d))
}

func (r *IPRanges) appendIP(ip net.IP) {
	r.ips = append(r.ips, &net.IPAddr{IP: ip})
	if r.currentPort > 0 {
		IPPortMap[ip.String()] = r.currentPort
		IPPortFromUser[ip.String()] = true // 标记用户显式指定了端口
	}
}

// 返回第四段 ip 的最小值及可用数目
func (r *IPRanges) getIPRange() (minIP, hosts byte) {
	minIP = r.firstIP[15] & r.ipNet.Mask[3] // IP 第四段最小值

	// 根据子网掩码获取主机数量
	m := net.IPv4Mask(255, 255, 255, 255)
	for i, v := range r.ipNet.Mask {
		m[i] ^= v
	}
	total, _ := strconv.ParseInt(m.String(), 16, 32) // 总可用 IP 数
	if total > 255 {                                 // 矫正 第四段 可用 IP 数
		hosts = 255
		return
	}
	hosts = byte(total)
	return
}

func (r *IPRanges) chooseIPv4() {
	if r.mask == "/32" { // 单个 IP 则无需随机，直接加入自身即可
		r.appendIP(r.firstIP)
	} else {
		minIP, hosts := r.getIPRange()    // 返回第四段 IP 的最小值及可用数目
		for r.ipNet.Contains(r.firstIP) { // 只要该 IP 没有超出 IP 网段范围，就继续循环随机
			if TestAll { // 如果是测速全部 IP
				for i := 0; i <= int(hosts); i++ { // 遍历 IP 最后一段最小值到最大值
					r.appendIPv4(byte(i) + minIP)
				}
			} else { // 随机 IP 的最后一段 0.0.0.X
				r.appendIPv4(minIP + randIPEndWith(hosts))
			}
			r.firstIP[14]++ // 0.0.(X+1).X
			if r.firstIP[14] == 0 {
				r.firstIP[13]++ // 0.(X+1).X.X
				if r.firstIP[13] == 0 {
					r.firstIP[12]++ // (X+1).X.X.X
				}
			}
		}
	}
}

func (r *IPRanges) chooseIPv6() {
	if r.mask == "/128" { // 单个 IP 则无需随机，直接加入自身即可
		r.appendIP(r.firstIP)
	} else {
		var tempIP uint8                  // 临时变量，用于记录前一位的值
		for r.ipNet.Contains(r.firstIP) { // 只要该 IP 没有超出 IP 网段范围，就继续循环随机
			r.firstIP[15] = randIPEndWith(255) // 随机 IP 的最后一段
			r.firstIP[14] = randIPEndWith(255) // 随机 IP 的最后一段

			targetIP := make([]byte, len(r.firstIP))
			copy(targetIP, r.firstIP)
			r.appendIP(targetIP) // 加入 IP 地址池

			for i := 13; i >= 0; i-- { // 从倒数第三位开始往前随机
				tempIP = r.firstIP[i]              // 保存前一位的值
				r.firstIP[i] += randIPEndWith(255) // 随机 0~255，加到当前位上
				if r.firstIP[i] >= tempIP {        // 如果当前位的值大于等于前一位的值，说明随机成功了，可以退出该循环
					break
				}
			}
		}
	}
}

// ipSourceEntry 一个 IP 来源条目（-ip 参数中的一段，或文件中的一行）
type ipSourceEntry struct {
	ipPart string // 剥离 "=数量" 后的 IP/CIDR 部分（可能仍含端口）
	count  int    // 用户指定的采样数量（CIDR=数量 语法），0 表示未指定
}

// cleanIPSourceLine 清理单个来源条目：去首尾空白，跳过空行与注释行（# 或 // 开头）
func cleanIPSourceLine(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || strings.HasPrefix(s, "#") || strings.HasPrefix(s, "//") {
		return ""
	}
	return s
}

// parseSourceEntry 解析单个来源条目，支持 "CIDR=数量" 采样语法
// 例如：2606:4700::/48=1000 表示对该网段均匀采样最多 1000 个 IP
// 单独 IP（/32、/128）指定的数量会被忽略（仅一个地址可测）
func parseSourceEntry(raw string) (ipSourceEntry, error) {
	entry := ipSourceEntry{ipPart: raw}
	idx := strings.IndexByte(raw, '=')
	if idx < 0 { // 无 "=数量" 部分
		return entry, nil
	}
	entry.ipPart = strings.TrimSpace(raw[:idx])
	if entry.ipPart == "" {
		return entry, fmt.Errorf("IP 段 [%s] 无效：缺少 IP/CIDR 部分", raw)
	}
	countStr := strings.TrimSpace(raw[idx+1:])
	count, err := strconv.Atoi(countStr)
	if err != nil || count <= 0 {
		return entry, fmt.Errorf("IP 段 [%s] 的采样数量 [%s] 无效（应为正整数，如 2606:4700::/48=1000）", raw, countStr)
	}
	entry.count = count
	return entry, nil
}

// collectIPSources 收集、清理并去重 IP 来源条目
// 优先级：-ip 参数 > -f 文件（与历史行为一致）；仅对生效的来源做去重
// 去重按清理后的原始文本匹配（保留首次出现顺序）
func collectIPSources() ([]ipSourceEntry, error) {
	var lines []string
	if IPText != "" { // 从参数中获取 IP 段数据（英文逗号分隔）
		lines = strings.Split(IPText, ",")
	} else { // 从文件中获取 IP 段数据
		if IPFile == "" {
			IPFile = defaultInputFile
		}
		if _, err := os.Stat(IPFile); err != nil {
			return nil, fmt.Errorf("IP 数据文件不存在: %s", IPFile)
		}
		file, err := os.Open(IPFile)
		if err != nil {
			return nil, fmt.Errorf("读取 IP 数据文件 [%s] 失败：%v", IPFile, err)
		}
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() { // 循环遍历文件每一行
			lines = append(lines, scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("读取 IP 数据文件 [%s] 失败：%v", IPFile, err)
		}
	}

	seen := make(map[string]struct{}, len(lines))
	entries := make([]ipSourceEntry, 0, len(lines))
	for _, line := range lines {
		clean := cleanIPSourceLine(line)
		if clean == "" { // 跳过空行、注释行（即开头、结尾或连续多个 ,, 的情况）
			continue
		}
		if _, dup := seen[clean]; dup { // 跳过重复条目（保序去重）
			continue
		}
		seen[clean] = struct{}{}
		entry, err := parseSourceEntry(clean)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// chooseIPv4Counted 对整个 IPv4 网段均匀分区间采样 count 个 IP：
// 将网段地址范围均分为 count 个区间，每个区间内随机取 1 个（区间互不重叠，结果天然无重复）
// 用户指定数量优先于 [-allip]；count 大于网段地址总数时钳制为总数（即测全部）
func (r *IPRanges) chooseIPv4Counted(count int) {
	if r.mask == "/32" { // 单个 IP 无需采样，直接加入自身
		r.appendIP(r.firstIP)
		return
	}
	ones, _ := r.ipNet.Mask.Size()
	size := uint64(1) << uint(32-ones) // 网段地址总数（最大 2^32，uint64 安全）
	if size < uint64(count) {          // 钳制：数量不超过网段地址总数（此时 size 必然可安全转 int）
		count = int(size)
	}
	start := binary.BigEndian.Uint32(r.firstIP.To4()) // 网段起始地址（ParseCIDR 结果已按掩码对齐）
	interval := size / uint64(count)                  // 每个区间长度（≥1）
	lastSize := size - interval*uint64(count-1)       // 最后一个区间的实际长度（补整除余数）
	for i := 0; i < count; i++ {
		var off uint64
		if i == count-1 { // 最后一个区间
			if lastSize > 1 {
				off = rand.Uint64() % lastSize
			}
		} else if interval > 1 {
			off = rand.Uint64() % interval
		}
		ip := uint64(start) + uint64(i)*interval + off // 恒 ≤ 网段末尾地址，不会溢出
		r.appendIP(net.IPv4(byte(ip>>24), byte(ip>>16), byte(ip>>8), byte(ip)))
	}
}

// chooseIPv6Counted 对整个 IPv6 网段均匀分区间采样 count 个 IP（同 chooseIPv4Counted）
// IPv6 地址范围为 128 位，使用 math/big 计算，随机偏移使用 crypto/rand 保证全区间均匀覆盖
func (r *IPRanges) chooseIPv6Counted(count int) {
	if r.mask == "/128" { // 单个 IP 无需采样，直接加入自身
		r.appendIP(r.firstIP)
		return
	}
	ones, _ := r.ipNet.Mask.Size()
	size := new(big.Int).Lsh(big.NewInt(1), uint(128-ones)) // 网段地址总数（最大 2^128）
	maxCount := uint64(^uint(0) >> 1)                       // int 平台最大值（兼容 32 位构建）
	if size.IsUint64() && size.Uint64() < maxCount && size.Uint64() < uint64(count) {
		count = int(size.Uint64()) // 钳制：数量不超过网段地址总数（小网段才可能命中）
	}
	start := new(big.Int).SetBytes(r.firstIP.To16()) // 网段起始地址（ParseCIDR 结果已按掩码对齐）
	countBig := big.NewInt(int64(count))
	interval := new(big.Int).Div(size, countBig) // 每个区间长度（≥1）
	lastSize := new(big.Int).Sub(size, new(big.Int).Mul(interval, big.NewInt(int64(count-1))))
	one := big.NewInt(1)
	for i := 0; i < count; i++ {
		mod := interval // 本区间长度（随机偏移的取值范围上限）
		if i == count-1 {
			mod = lastSize
		}
		var off *big.Int
		if mod.Cmp(one) > 0 { // 区间长度 > 1 才需要随机，crypto/rand.Int 支持 2^128 内任意模数
			off, _ = crand.Int(crand.Reader, mod)
		} else {
			off = big.NewInt(0)
		}
		ipBig := new(big.Int).Add(start, new(big.Int).Add(new(big.Int).Mul(big.NewInt(int64(i)), interval), off))
		r.appendIP(net.IP(ipBig.FillBytes(make([]byte, 16))))
	}
}

func loadIPRanges() []*net.IPAddr {
	ranges := newIPRanges()
	sources, err := collectIPSources()
	if err != nil {
		log.Fatal(err)
	}
	if len(sources) == 0 { // 来源清理后一条不剩（如文件全是注释/空行），明确报错而非静默空跑
		log.Fatal("未获取到任何 IP 或 CIDR，请检查 [-ip] 参数或 IP 数据文件内容")
	}
	for _, source := range sources {
		ranges.parseCIDR(source.ipPart) // 解析 IP 段，获得 IP、IP 范围、子网掩码、端口
		if source.count > 0 {           // 指定了采样数量（CIDR=数量），均匀分区间采样
			if isIPv4(source.ipPart) {
				ranges.chooseIPv4Counted(source.count)
			} else {
				ranges.chooseIPv6Counted(source.count)
			}
			continue
		}
		if isIPv4(source.ipPart) { // 生成要测速的所有 IPv4 / IPv6 地址（单个/随机/全部）
			ranges.chooseIPv4()
		} else {
			ranges.chooseIPv6()
		}
	}
	return ranges.ips
}
