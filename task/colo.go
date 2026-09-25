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
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/masgzy/CloudflareST/utils"
)

// ForceGetColo 是否强制获取机场三字码（-getcolo 控制）
// 适用场景：TCPing + -dd 时延迟测速与下载测速均不产生 HTTP 请求，
// 结果中的地区码只能显示 N/A；开启后会对测速结果逐个发起轻量 HTTP HEAD 请求补齐地区码。
// 注意：HTTPing 模式本身就通过响应头获取地区码，无需也不应开启本参数。
var ForceGetColo bool

// getColoUserAgent 与延迟/下载测速保持一致的 UA
const getColoUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_12_6) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/98.0.4758.80 Safari/537.36"

// TestGetColo 为结果集中地区码仍为空的 IP 补齐机场三字码。
// 实现：向 [-url] 发起一次 HEAD 请求（直连该 IP），从响应头（如 cf-ray）解析地区码。
// 请求不参与任何延迟/速度计算，仅用于获取地区码，因此不影响测速结果数值。
// 返回值与入参是同一份切片（原地写入 Colo 字段），便于在 main 中链式使用。
func TestGetColo(ipSet utils.PingDelaySet) utils.PingDelaySet {
	if !ForceGetColo || Httping {
		return ipSet
	}
	// 只对地区码为空的结果发起请求（HTTPing 模式下不会为空，已在上方直接返回）
	targets := make([]*utils.CloudflareIPData, 0, len(ipSet))
	for i := range ipSet {
		if ipSet[i].Colo == "" {
			targets = append(targets, &ipSet[i])
		}
	}
	if len(targets) == 0 {
		return ipSet
	}

	fmt.Printf("开始获取地区码（数量：%d, 并发：%d, 地址：%s）\n", len(targets), Routines, URL)
	bar := utils.NewPingBar(len(targets))
	sem := newWeightedSemaphore(int64(Routines))
	wg := &sync.WaitGroup{}
	var completed int32

	for _, target := range targets {
		// 程序超时等场景下尽快收敛：已测得的延迟/速度结果不受影响，只是地区码可能仍为 N/A
		if atomic.LoadInt32(&GlobalEarlyStop) == 1 {
			break
		}
		// context.Background 永不取消，Acquire 只会在拿到配额后返回 nil，错误可安全忽略
		_ = sem.Acquire(context.Background(), 1)
		wg.Add(1)
		go func(t *utils.CloudflareIPData) {
			defer wg.Done()
			defer sem.Release(1)
			t.Colo = getColoHandler(t.IP)
			n := atomic.AddInt32(&completed, 1)
			bar.Update(int(n), fmt.Sprintf("%d/%d", n, len(targets)), "")
		}(target)
	}
	wg.Wait()
	bar.Done()
	// -cfcolo 过滤：HTTPing 模式在延迟测速阶段已过滤；TCPing 模式的地区码由本函数补齐，
	// 因此在此处统一过滤。获取失败的空地区码同样被移除（与 HTTPing 下无法解析地区码的处理一致）。
	if HttpingCFColomap != nil {
		filtered := make(utils.PingDelaySet, 0, len(ipSet))
		for i := range ipSet {
			if ipSet[i].Colo == "" {
				continue
			}
			if _, ok := HttpingCFColomap.Load(ipSet[i].Colo); ok {
				filtered = append(filtered, ipSet[i])
			}
		}
		fmt.Printf("地区码过滤（[-cfcolo]）：%d → %d\n", len(ipSet), len(filtered))
		return filtered
	}
	return ipSet
}

// getColoHandler 单个 IP 的地区码获取：HEAD 请求直连该 IP，从响应头解析地区码
func getColoHandler(ip *net.IPAddr) string {
	hc := http.Client{
		Timeout: time.Second * 2,
		Transport: &http.Transport{
			DialContext: getDialContext(ip),
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // 阻止重定向（301/302 响应头同样包含地区码）
		},
	}
	defer hc.CloseIdleConnections()

	request, err := http.NewRequest(http.MethodHead, URL, nil)
	if err != nil {
		return ""
	}
	request.Header.Set("User-Agent", getColoUserAgent)
	response, err := hc.Do(request)
	if err != nil {
		return ""
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	return getHeaderColo(response.Header)
}
