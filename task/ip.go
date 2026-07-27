package task

import (
	"bufio"
	"log"
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
	// Go 1.20+ 已自动播种全局 rand，但为兼容 Go 1.18（go.mod 要求）仍需手动播种。
	// 在 Go 1.20+ 中此调用会被忽略（仅产生 deprecation 提示），不影响功能。
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
func extractPort(ip string) (cleanIP string, port int) {
	port = 0 // 0 表示使用全局 TCPPort
	cleanIP = ip

	// IPv6 格式（含 [] 包裹）: [::1]:443 或 [2606:4700::/32]:443
	if strings.HasPrefix(ip, "[") {
		if idx := strings.LastIndex(ip, "]:"); idx > 0 {
			p, err := strconv.Atoi(ip[idx+2:])
			if err == nil && p > 0 && p < 65536 {
				port = p
				cleanIP = ip[:idx+1] // 保留 [...] 部分
				return
			}
		}
		return // [::1] 或 [2606:4700::/32]，无端口
	}

	// IPv4 网段含端口: 1.1.1.0/24:443
	if slashIdx := strings.Index(ip, "/"); slashIdx > 0 {
		if colonIdx := strings.Index(ip[slashIdx:], ":"); colonIdx > 0 {
			p, err := strconv.Atoi(ip[slashIdx+colonIdx+1:])
			if err == nil && p > 0 && p < 65536 {
				port = p
				cleanIP = ip[:slashIdx+colonIdx]
				return
			}
		}
		return // 1.1.1.0/24，无端口
	}

	// 单 IP（无 /）: 1.1.1.1:443 或 1.1.1.1
	// 注意 IPv6 不含 [] 时可能有多个冒号（::1），仅当恰好一个冒号时视作 IPv4:端口
	colonCount := strings.Count(ip, ":")
	if colonCount == 1 {
		lastColon := strings.LastIndex(ip, ":")
		p, err := strconv.Atoi(ip[lastColon+1:])
		if err == nil && p > 0 && p < 65536 {
			port = p
			cleanIP = ip[:lastColon]
		}
	}
	// colonCount==0: 纯 IPv4，无端口；colonCount>1: IPv6，无端口
	return
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

func loadIPRanges() []*net.IPAddr {
	ranges := newIPRanges()
	if IPText != "" { // 从参数中获取 IP 段数据
		IPs := strings.Split(IPText, ",") // 以逗号分隔为数组并循环遍历
		for _, IP := range IPs {
			IP = strings.TrimSpace(IP) // 去除首尾的空白字符（空格、制表符、换行符等）
			if IP == "" {              // 跳过空的（即开头、结尾或连续多个 ,, 的情况）
				continue
			}
			ranges.parseCIDR(IP) // 解析 IP 段，获得 IP、IP 范围、子网掩码
			if isIPv4(IP) {      // 生成要测速的所有 IPv4 / IPv6 地址（单个/随机/全部）
				ranges.chooseIPv4()
			} else {
				ranges.chooseIPv6()
			}
		}
	} else { // 从文件中获取 IP 段数据
		if IPFile == "" {
			IPFile = defaultInputFile
		}
		file, err := os.Open(IPFile)
		if err != nil {
			log.Fatal(err)
		}
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() { // 循环遍历文件每一行
			line := strings.TrimSpace(scanner.Text()) // 去除首尾的空白字符（空格、制表符、换行符等）
			if line == "" {                           // 跳过空行
				continue
			}
			ranges.parseCIDR(line) // 解析 IP 段，获得 IP、IP 范围、子网掩码
			if isIPv4(line) {      // 生成要测速的所有 IPv4 / IPv6 地址（单个/随机/全部）
				ranges.chooseIPv4()
			} else {
				ranges.chooseIPv6()
			}
		}
		if err := scanner.Err(); err != nil {
			log.Fatalf("读取 IP 数据文件 [%s] 失败：%v", IPFile, err)
		}
	}
	return ranges.ips
}
