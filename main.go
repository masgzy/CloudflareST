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

package main

import (
	"bufio"
	"context"
	_ "embed"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"github.com/masgzy/CloudflareST/pkg/update"
	"github.com/masgzy/CloudflareST/task"
	"github.com/masgzy/CloudflareST/utils"
)

//go:embed statement.txt
var licenseContent string

var (
	version string
)

func init() {

	var printVersion bool
	var useIPv6 bool
	var help = `
\x1b[34;1m# CloudflareST\x1b[0m ` + version + ` 本项目基于XIU2/CloudflareSpeedTest进行修改，使用GPL3.0协议开源，测试各个 CDN 或网站所有 IP 的延迟和速度，获取最快 IP (IPv4+IPv6)！https://github.com/masgzy/CloudflareST

参数：

  目标参数：
    -f ip.txt
        IP段数据文件；如路径含有空格请加上引号；支持其他 CDN IP段；(默认 ip.txt)
        支持 # 与 // 开头的注释行，重复的 IP 段会自动去重；同样支持端口与「网段=数量」写法
    -ip 1.1.1.1,2.2.2.2/24,2606:4700::/32
        指定IP段数据；直接通过参数指定要测速的 IP 段数据，英文逗号分隔；(默认 空)
        支持指定端口：单个IP → 1.1.1.1:443、IPv4网段 → 1.1.1.0/24:443、IPv6 → [::1]:443、IPv6网段 → [2606:4700::/32]:443
        指定端口时不要带 /32 后缀，程序会自动处理
        支持指定数量：网段=数量 → 2606:4700::/48=1000，对该网段均匀采样最多 1000 个 IP（搭配 -tn 更高效）
    -ipv6
        使用自带的 ipv6.txt 数据文件；等效于 [-f ipv6.txt]（与 -f 同时指定时优先 -f）
    -url https://cf.xiu2.xyz/url
        指定测速地址；延迟测速(HTTPing)/下载测速时使用的地址，默认地址不保证可用性，建议自建；
        需以 http:// 或 https:// 协议前缀开头
    -tp 443
        指定测速端口；延迟测速/下载测速时使用的端口；(默认 443 端口)
        -httping 模式下若未显式指定 -tp，会按测速地址协议自动选择（http → 80、https → 443）

  测试参数：
    -n 200
        延迟测速线程；越多延迟测速越快，性能弱的设备 (如路由器) 请勿太高；(默认 200 无上限)
    -t 4
        延迟测速次数；单个 IP 延迟测速的次数；(默认 4 次)
    -tn 0
        延迟测速可用数量；当可用IP数量达到此值时提前结束延迟测速，0 表示不限制；(默认 0 不限制)
    -dn 10
        下载测速数量；延迟测速并排序后，从最低延迟起下载测速的数量；(默认 10 个)
    -dt 10
        下载测速时间；单个 IP 下载测速最长时间，不能太短；(默认 10 秒)
    -httping
        切换测速模式；延迟测速模式改为 HTTP 协议，所用测试地址为 [-url] 参数；(默认 TCPing)
    -httping-code 200
        有效状态代码；HTTPing 延迟测速时网页返回的有效 HTTP 状态码，仅限一个；(默认 200 301 302)
    -cfcolo HKG,KHH,NRT,LAX,SEA,SJC,FRA,MAD
        匹配指定地区；IATA 机场地区码或国家/城市码，英文逗号分隔，仅 HTTPing 模式可用；(默认 所有地区)
    -dd
        禁用下载测速；禁用后测速结果会按延迟排序 (默认按下载速度排序)；(默认 启用)
    -getcolo
        强制获取机场三字码；TCPing 模式下结果默认显示 N/A，开启后会对测速结果
        逐个发起轻量 HEAD 请求（不计入测速结果）补齐地区码，搭配 [-dd] 使用效果最佳；
        HTTPing 模式无需开启（本身已获取）；地区码仅用于展示，不参与 [-cfcolo] 过滤；(默认 关闭)
    -allip
        测速全部的IP；对 IP 段中的每个 IP (仅支持 IPv4) 进行测速；(默认 每个 /24 段随机测速一个 IP)
        （指定了「网段=数量」的条目不受此参数影响，会按指定数量采样）
    -pi 0
        每次 ping 间隔；单个 IP 每次延迟测速之间的间隔时间（毫秒），0 表示不间隔；若测速后 IP 不可用可尝试设置 100-200；(默认 0)
    -intf eth0
        绑定网络接口；绑定到指定的网络接口名或本地 IP 进行测速，如 eth0、pppoe-ct 或 192.168.1.100；(默认 空)
    -timeout 3600
        程序超时退出；程序运行超时时间（秒），超时后立即结算结果并退出；(默认 0 不限制)

  过滤参数：
    -tl 200
        平均延迟上限；只输出低于指定平均延迟的 IP，各上下限条件可搭配使用；(默认 9999 ms)
    -tll 40
        平均延迟下限；只输出高于指定平均延迟的 IP；高于 [-tl] 时会自动调整至上限；(默认 0 ms)
    -tlr 0.2
        丢包几率上限；只输出低于/等于指定丢包率的 IP，范围 0.00~1.00，0 过滤掉任何丢包的 IP；(默认 1.00)
    -sl 5
        下载速度下限；只输出高于指定下载速度的 IP，凑够指定数量 [-dn] 才会停止测速；(默认 0.00 MB/s)

  结果参数：
    -p 10
        显示结果数量；测速后直接显示指定数量的结果，为 0 时不显示结果直接退出；(默认 10 个)
    -o result.csv
        写入结果文件；如路径含有空格请加上引号；值为空时不写入文件 [-o ""]；(默认 result.csv)
    -sp
        显示端口号；在结果中显示测速端口（IP:PORT），默认仅当用户指定端口时显示；(默认 关闭)
    -zs
        综合排序模式；下载测速结果按速度+延迟+丢包率的综合评分排序（而非纯速度排序）；(默认 关闭)

  其他参数：
    -debug
        调试输出模式；会在一些非预期情况下输出更多日志以便判断原因；(默认 关闭)

    -v
        打印程序版本 + 检查版本更新
    -h
        打印帮助说明
`
	var minDelay, maxDelay, downloadTime int
	var maxLossRate float64
	var programTimeout int
	var pingInterval int
	flag.IntVar(&task.Routines, "n", 200, "延迟测速线程")
	flag.IntVar(&task.PingTimes, "t", 4, "延迟测速次数")
	flag.IntVar(&task.TargetNum, "tn", 0, "延迟测速可用数量")
	flag.IntVar(&task.TestCount, "dn", 10, "下载测速数量")
	flag.IntVar(&downloadTime, "dt", 10, "下载测速时间")
	flag.IntVar(&task.TCPPort, "tp", 443, "指定测速端口")
	flag.StringVar(&task.URL, "url", "https://cf.xiu2.xyz/url", "指定测速地址")

	flag.BoolVar(&task.Httping, "httping", false, "切换测速模式")
	flag.IntVar(&task.HttpingStatusCode, "httping-code", 0, "有效状态代码")
	flag.StringVar(&task.HttpingCFColo, "cfcolo", "", "匹配指定地区")

	flag.IntVar(&maxDelay, "tl", 9999, "平均延迟上限")
	flag.IntVar(&minDelay, "tll", 0, "平均延迟下限")
	flag.Float64Var(&maxLossRate, "tlr", 1, "丢包几率上限")
	flag.Float64Var(&task.MinSpeed, "sl", 0, "下载速度下限")

	flag.IntVar(&utils.PrintNum, "p", 10, "显示结果数量")
	flag.StringVar(&task.IPFile, "f", "ip.txt", "IP段数据文件")
	flag.BoolVar(&useIPv6, "ipv6", false, "使用 ipv6.txt 数据文件")
	flag.StringVar(&task.IPText, "ip", "", "指定IP段数据")
	flag.StringVar(&utils.Output, "o", "result.csv", "输出结果文件")

	flag.BoolVar(&task.Disable, "dd", false, "禁用下载测速")
	flag.BoolVar(&task.ForceGetColo, "getcolo", false, "强制获取机场三字码")
	flag.BoolVar(&task.TestAll, "allip", false, "测速全部 IP")
	flag.BoolVar(&utils.ShowPort, "sp", false, "显示端口号")
	flag.BoolVar(&utils.UseZScore, "zs", false, "综合排序模式")

	flag.StringVar(&task.BindIntf, "intf", "", "绑定网络接口")
	flag.IntVar(&programTimeout, "timeout", 0, "程序超时退出")
	flag.IntVar(&pingInterval, "pi", 0, "每次 ping 间隔")

	flag.BoolVar(&utils.Debug, "debug", false, "调试输出模式")

	flag.BoolVar(&printVersion, "v", false, "打印程序版本")
	flag.Usage = func() { fmt.Print(strings.ReplaceAll(help, "\\x1b", "\x1b")) }
	flag.Parse()

	// 检测哪些参数被显式指定（用于 -ipv6 优先级、-tp 自动端口、-dd 与 -url 冲突提醒）
	ipFileSet, tpSet, urlSet := false, false, false
	flag.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "f":
			ipFileSet = true
		case "tp":
			tpSet = true
		case "url":
			urlSet = true
		}
	})
	handleIPFlags(useIPv6, ipFileSet)

	// -tll 高于 -tl 时自动钳制到上限，避免上下限矛盾导致过滤结果全为空
	if minDelay > maxDelay {
		utils.Tip("平均延迟下限 [-tll] 高于上限 [-tl]，已自动调整为上限值。")
		minDelay = maxDelay
	}

	// 注意：此处比较的是 utils.InputMaxDelay 的默认值（尚未被下方的赋值覆盖），
	// 用于判断用户是否未指定 -tl 参数（即仍为默认 9999ms）。
	if task.MinSpeed > 0 && time.Duration(maxDelay)*time.Millisecond == utils.InputMaxDelay {
		utils.Tip("在使用 [-sl] 参数时，建议搭配 [-tl] 参数，以避免因凑不够 [-dn] 数量而一直测速...")
	}
	utils.InputMaxDelay = time.Duration(maxDelay) * time.Millisecond
	utils.InputMinDelay = time.Duration(minDelay) * time.Millisecond
	utils.InputMaxLossRate = float32(maxLossRate)
	task.Timeout = time.Duration(downloadTime) * time.Second
	task.PingInterval = time.Duration(pingInterval) * time.Millisecond
	task.HttpingCFColomap = task.MapColoMap()
	task.ProgramTimeout = programTimeout

	// -httping 模式下未显式指定 -tp 时，按测速地址协议自动选择端口（http→80、https→443）
	// 避免用户用 http:// 地址测速却忘改 -tp 80，导致全部请求撞上 TLS 握手失败
	if task.Httping && !tpSet {
		if strings.HasPrefix(task.URL, "http://") && task.TCPPort != 80 {
			task.TCPPort = 80
			utils.Tip("已根据 [-httping] 测速地址的 HTTP 协议自动选择测速端口 80（可用 [-tp] 覆盖）。")
		}
		// https 默认端口即为 443，无需处理
	}

	if printVersion {
		println(version)
		fmt.Println("检查版本更新中...")
		status, checkErr := update.CheckUpdate(version)
		switch status {
		case update.CheckUpToDate:
			utils.Green.Println("当前为最新版本 [" + version + "]！")
		case update.CheckHasUpdate:
			utils.Yellow.Printf("*** 发现新版本 [%s]！是否立即更新？[Y/n] ***\n", update.LatestVersion)
			reader := bufio.NewReader(os.Stdin)
			ans, _ := reader.ReadString('\n')
			ans = strings.TrimSpace(strings.ToLower(ans))
			if ans != "" && ans != "y" && ans != "yes" {
				fmt.Println("已跳过更新。")
				os.Exit(0)
			}
			if err := update.PerformUpdate(context.Background()); err != nil {
				utils.Error("更新失败: %v", err)
				fmt.Println("可前往 https://github.com/masgzy/CloudflareST/releases/latest 手动下载。")
				os.Exit(1)
			}
			utils.Green.Println("更新完成！请重新启动程序。")
		default: // update.CheckFailed
			// 绝不能把"检查失败"说成"已是最新"，明确告知用户原因并给出手动检查途径
			utils.Error("检查更新失败（%v）。", checkErr)
			fmt.Printf("当前版本 [%s]，可稍后重试，或前往 https://github.com/masgzy/CloudflareST/releases/latest 手动检查新版本。\n", version)
		}
		os.Exit(0)
	}

	// 参数校验（-v 模式无需校验测速参数，已在上方退出）
	// 地址是否会被使用：HTTPing 模式用于延迟测速；未禁用下载测速时用于下载测速；
	// -getcolo 模式下即使 TCPing + -dd 也会用它发起地区码获取请求
	if task.Httping || !task.Disable || task.ForceGetColo {
		if err := task.ValidateTestURL(task.URL); err != nil {
			utils.Error("%v", err)
			os.Exit(1)
		}
		if task.PortProtocolMismatch(task.TCPPort, task.URL) {
			utils.Tip("指定的测速端口 [-tp] 与测速地址协议可能不匹配（TLS 握手将失败），请注意确认。")
		}
	} else if urlSet {
		// 已禁用下载测速且非 HTTPing 模式时，-url 不会被用到，提醒用户避免误解
		utils.Tip("使用了 [-dd] 参数，[-url] 指定的测速地址不会被使用。")
	}

	// -getcolo 参数合理性提醒
	if task.ForceGetColo {
		if task.Httping {
			utils.Tip("[-httping] 模式本身已通过响应头获取地区码，[-getcolo] 参数无实际作用。")
		} else if task.Disable {
			utils.Tip("已开启 [-getcolo]：测速结束后将向 [-url] 发起轻量请求以获取地区码。")
		}
	}
}

// handleIPFlags 处理 -ipv6 与 -f 的关系：
// -f 显式指定时优先于 -ipv6（更具体的意图优先），并在两者冲突时提示用户
func handleIPFlags(useIPv6, ipFileSet bool) {
	if useIPv6 {
		if ipFileSet {
			utils.Tip("已同时指定 [-ipv6] 与 [-f]，优先使用 [-f] 指定的 IP 文件。")
			return
		}
		task.IPFile = "ipv6.txt"
	}
}

func main() {
	// 首次运行检测，输出 license 内容
	checkFirstRun()

	task.InitRandSeed()     // 置随机数种子
	task.ValidateBindIntf() // 验证绑定接口参数是否有效

	fmt.Printf("\x1b[34;1m# CloudflareST\x1b[0m %s\n", version)

	// 如果设置了程序超时时间，启动超时处理 goroutine
	if task.ProgramTimeout > 0 {
		fmt.Printf("程序超时时间: %d 秒\n", task.ProgramTimeout)
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(task.ProgramTimeout)*time.Second)
		defer cancel()
		go func() {
			<-ctx.Done()
			if ctx.Err() != context.DeadlineExceeded {
				return // main 正常结束时 cancel 触发，不是超时，直接退出 goroutine
			}
			fmt.Print("\r\x1b[K")
			utils.Info("程序运行超时，正在结算结果并退出...")
			// 置停止标志后，测速各环节（延迟测速循环/下载测速循环）都会尽快收敛，
			// 主流程会继续走完 ExportCsv + Print，保证已测得的结果不丢失。
			atomic.StoreInt32(&task.GlobalEarlyStop, 1)
			// 兑底看门狗：正常情况下主流程会先自然退出；万一卡死则强制退出，避免 -timeout 失效。
			// 时间取 下载测速超时的 2 倍 + 30 秒，且不少于 2 分钟。
			watchdog := 2*task.Timeout + 30*time.Second
			if watchdog < 2*time.Minute {
				watchdog = 2 * time.Minute
			}
			time.Sleep(watchdog)
			utils.Warn("结算超时，强制退出（结果可能未完整写入）。")
			os.Exit(0)
		}()
	}

	// 如果设置了绑定接口，输出提示
	if task.BindIntf != "" {
		fmt.Printf("绑定网络接口: %s\n", task.BindIntf)
	}

	// 开始延迟测速 + 过滤延迟/丢包
	pingData := task.NewPing().Run().FilterDelay().FilterLossRate()
	// 强制获取机场三字码（-getcolo，仅 TCPing 模式生效；TCPing 默认不产生 HTTP 请求，地区码为空）
	pingData = task.TestGetColo(pingData)
	// 开始下载测速
	speedData := task.TestDownloadSpeed(pingData)
	utils.ExportCsv(speedData) // 输出文件
	speedData.Print()          // 打印结果
	endPrint()                 // 根据情况选择退出方式（针对 Windows）
}

// 根据情况选择退出方式（针对 Windows）
func endPrint() {
	if utils.NoPrintResult() { // 如果不需要打印测速结果，则直接退出
		return
	}
	if runtime.GOOS == "windows" { // 如果是 Windows 系统，则需要按下 回车键 或 Ctrl+C 退出（避免通过双击运行时，测速完毕后直接关闭）
		fmt.Printf("按下 回车键 或 Ctrl+C 退出。")
		_, _ = fmt.Scanln()
	}
}

// 首次运行检测，输出 license 内容并删除标记文件
func checkFirstRun() {
	firstRunFile := ".first_run"
	if _, err := os.Stat(firstRunFile); err == nil {
		// .first_run 文件存在，输出 license 内容（支持 ANSI 彩色转义）
		// 处理 ANSI 转义序列，将 \x1b 替换为实际转义字符
		content := strings.ReplaceAll(licenseContent, "\\x1b", "\x1b")
		fmt.Println(content)
		// 删除 .first_run 文件
		os.Remove(firstRunFile)
	}
}
