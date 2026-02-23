package task

import (
	//"crypto/tls"

	"io"
	"log"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/XIU2/CloudflareSpeedTest/utils"
)

var (
	Httping               bool
	HttpingStatusCode     int
	HttpingCFColo         string
	HttpingCFColomap      *sync.Map
	RegexpColoIATACode    = regexp.MustCompile(`[A-Z]{3}`)  // 匹配 IATA 机场地区码的正则表达式
	RegexpColoCountryCode = regexp.MustCompile(`[A-Z]{2}`)  // 匹配国家地区码的正则表达式
	RegexpColoGcore       = regexp.MustCompile(`^[a-z]{2}`) // 匹配城市地区码的正则表达式（小写）
)

// pingReceived pingTotalTime
func (p *Ping) httping(ip *net.IPAddr) (int, time.Duration, string) {
	hc := http.Client{
		Timeout: time.Second * 2,
		Transport: &http.Transport{
			DialContext: getDialContext(ip),
			//TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // 跳过证书验证
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // 阻止重定向
		},
	}

	// 先访问一次获得 HTTP 状态码及地区码
	var colo string
	{
		// 检查是否需要提前停止
		if atomic.LoadInt32(&p.earlyStop) == 1 {
			return 0, 0, ""
		}
		request, err := http.NewRequest(http.MethodHead, URL, nil)
		if err != nil {
			if utils.Debug { // 调试模式下，输出更多信息
				utils.Red.Printf("[调试] IP: %s, 延迟测速请求创建失败，错误信息: %v, 测速地址: %s\n", ip.String(), err, URL)
			}
			return 0, 0, ""
		}
		request.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_12_6) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/98.0.4758.80 Safari/537.36")
		response, err := hc.Do(request)
		if err != nil {
			if utils.Debug { // 调试模式下，输出更多信息
				utils.Red.Printf("[调试] IP: %s, 延迟测速失败，错误信息: %v, 测速地址: %s\n", ip.String(), err, URL)
			}
			return 0, 0, ""
		}
		defer response.Body.Close()

		// 如果未指定的 HTTP 状态码，或指定的状态码不合规，则默认只认为 200、301、302 才算 HTTPing 通过
		if HttpingStatusCode == 0 || HttpingStatusCode < 100 && HttpingStatusCode > 599 {
			if response.StatusCode != 200 && response.StatusCode != 301 && response.StatusCode != 302 {
				if utils.Debug { // 调试模式下，输出更多信息
					utils.Red.Printf("[调试] IP: %s, 延迟测速终止，HTTP 状态码: %d, 测速地址: %s\n", ip.String(), response.StatusCode, URL)
				}
				return 0, 0, ""
			}
		} else {
			if response.StatusCode != HttpingStatusCode {
				if utils.Debug { // 调试模式下，输出更多信息
					utils.Red.Printf("[调试] IP: %s, 延迟测速终止，HTTP 状态码: %d, 指定的 HTTP 状态码: %d, 测速地址: %s\n", ip.String(), response.StatusCode, HttpingStatusCode, URL)
				}
				return 0, 0, ""
			}
		}

		io.Copy(io.Discard, response.Body)

		// 通过头部参数获取地区码
		colo = getHeaderColo(response.Header)

		// 只有指定了地区才匹配机场地区码
		if HttpingCFColo != "" {
			// 判断是否匹配指定的地区码
			colo = p.filterColo(colo)
			if colo == "" { // 没有匹配到地区码或不符合指定地区则直接结束该 IP 测试
				if utils.Debug { // 调试模式下，输出更多信息
					utils.Red.Printf("[调试] IP: %s, 地区码不匹配: %s\n", ip.String(), colo)
				}
				return 0, 0, ""
			}
		}
	}

	// 循环测速计算延迟
	success := 0
	var delay time.Duration
	for i := 0; i < PingTimes; i++ {
		// 在每次请求前检查是否需要提前停止
		if atomic.LoadInt32(&p.earlyStop) == 1 {
			return success, delay, colo
		}
		request, err := http.NewRequest(http.MethodHead, URL, nil)
		if err != nil {
			log.Fatal("意外的错误，请报告：", err)
			return 0, 0, ""
		}
		request.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_12_6) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/98.0.4758.80 Safari/537.36")
		if i == PingTimes-1 {
			request.Header.Set("Connection", "close")
		}
		startTime := time.Now()
		response, err := hc.Do(request)
		if err != nil {
			continue
		}
		success++
		io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
		duration := time.Since(startTime)
		delay += duration
	}

	return success, delay, colo
}

func MapColoMap() *sync.Map {
	if HttpingCFColo == "" {
		return nil
	}
	// 将 -cfcolo 参数指定的地区码转为大写并格式化
	colos := strings.Split(strings.ToUpper(HttpingCFColo), ",")
	colomap := &sync.Map{}
	for _, colo := range colos {
		colomap.Store(colo, colo)
	}
	return colomap
}

// 从响应头中获取地区码
func getHeaderColo(header http.Header) (colo string) {
	if header.Get("server") != "" {
		// 如果是 Cloudflare CDN
		// server: cloudflare
		// cf-ray: 7bd32409eda7b020-SJC
		if header.Get("server") == "cloudflare" {
			if colo = header.Get("cf-ray"); colo != "" {
				return RegexpColoIATACode.FindString(colo)
			}
		}
		// 如果是 CDN77 CDN
		// server: CDN77-Turbo
		// x-77-pop: losangelesUSCA
		// x-77-pop: frankfurtDE
		// x-77-pop: amsterdamNL
		// x-77-pop: singaporeSG
		if header.Get("server") == "CDN77-Turbo" {
			if colo = header.Get("x-77-pop"); colo != "" {
				return RegexpColoCountryCode.FindString(colo)
			}
		}
		// 如果是 Bunny CDN
		// server: BunnyCDN-TW1-1121
		if colo = header.Get("server"); strings.Contains(colo, "BunnyCDN-") {
			return RegexpColoCountryCode.FindString(strings.TrimPrefix(colo, "BunnyCDN-")) // 去掉 BunnyCDN- 前缀再去匹配
		}
	}
	// 如果是 AWS CloudFront CDN
	// x-amz-cf-pop: SIN52-P1
	if colo = header.Get("x-amz-cf-pop"); colo != "" {
		return RegexpColoIATACode.FindString(colo)
	}
	// 如果是 Fastly CDN
	// x-served-by: cache-qpg1275-QPG
	// x-served-by: cache-fra-etou8220141-FRA, cache-hhr-khhr2060043-HHR（最后一个为实际位置）
	if colo = header.Get("x-served-by"); colo != "" {
		if matches := RegexpColoIATACode.FindAllString(colo, -1); len(matches) > 0 {
			return matches[len(matches)-1] // 因为 Fastly 的 x-served-by 可能包含多个地区码，所以只取最后一个
		}
	}
	// Gcore CDN 的头部信息（注意均为城市代码而非国家代码）
	// x-id-fe: fr5-hw-edge-gc17
	// x-shard: fr5-shard0-default
	// x-id: fr5-hw-edge-gc28
	if colo = header.Get("x-id-fe"); colo != "" {
		if colo = RegexpColoGcore.FindString(colo); colo != "" {
			return strings.ToUpper(colo) // 将小写的地区码转换为大写
		}
	}

	// 如果没有获取到头部信息，说明不是支持的 CDN，则直接返回空字符串
	return ""
}

// 处理地区码
func (p *Ping) filterColo(colo string) string {
	if colo == "" {
		return ""
	}
	// 如果没有指定 -cfcolo 参数，则直接返回
	if HttpingCFColomap == nil {
		return colo
	}
	// 匹配机场地区码是否为指定的地区
	_, ok := HttpingCFColomap.Load(colo)
	if ok {
		return colo
	}
	return ""
}
