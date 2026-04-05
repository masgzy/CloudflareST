package task

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	cfIPDir        = "cfips"
	cfIPv4File     = "v4.txt"
	cfIPv6File     = "v6.txt"
	cfIPv4URL      = "https://www.cloudflare-cn.com/ips-v4/"
	cfIPv6URL      = "https://www.cloudflare-cn.com/ips-v6/"
	cfipsUserAgent = "CloudflareST"
)

func CFIPFilePath(useIPv6 bool) string {
	if useIPv6 {
		return filepath.Join(cfIPDir, cfIPv6File)
	}
	return filepath.Join(cfIPDir, cfIPv4File)
}

func EnsureCFIPFile(useIPv6 bool) (bool, error) {
	filePath := CFIPFilePath(useIPv6)
	if _, err := os.Stat(filePath); err != nil {
		if os.IsNotExist(err) {
			if err := UpdateCFIPs(); err != nil {
				return false, fmt.Errorf("未找到本地 Cloudflare 中国 IP 段文件，自动获取失败: %w", err)
			}
			return true, nil
		}
		return false, err
	}
	return false, nil
}

func ClearCFIPs() error {
	if err := os.Remove(CFIPFilePath(false)); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Remove(CFIPFilePath(true)); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Remove(cfIPDir); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func UpdateCFIPs() error {
	if err := os.MkdirAll(cfIPDir, 0o755); err != nil {
		return err
	}
	if err := downloadCFIPs(cfIPv4URL, CFIPFilePath(false)); err != nil {
		return err
	}
	if err := downloadCFIPs(cfIPv6URL, CFIPFilePath(true)); err != nil {
		return err
	}
	return nil
}

func downloadCFIPs(url, filePath string) error {
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", cfipsUserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("请求 %s 失败，HTTP 状态码：%d", url, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	content := strings.TrimSpace(string(body))
	if content == "" {
		return fmt.Errorf("请求 %s 返回内容为空", url)
	}

	content += "\n"
	return os.WriteFile(filePath, []byte(content), 0o644)
}
