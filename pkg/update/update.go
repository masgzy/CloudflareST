// CloudflareST - Cloudflare CDN IP 延迟和速度测试工具
// Copyright (C) 2020-2026 masgzy <https://github.com/masgzy>
//
// 本程序使用 GPL-3.0 协议开源。
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

package update

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/masgzy/CloudflareST/pkg/platform"
)

// GitHubProxy GitHub 代理默认前缀。
// 远端 version.txt 与 release 资产下载都通过该代理，避开 GFW 直连 GitHub 不稳定。
// 可通过环境变量 CFST_GITHUB_PROXY 覆盖（设 "-" 或 "off" 表示禁用代理直连）。
const GitHubProxy = "https://github.cnxiaobai.com/"

// proxyDisabledSentinels 用户填这些值表示禁用代理、直连 GitHub
var proxyDisabledSentinels = map[string]struct{}{
	"-":   {},
	"off": {},
	"no":  {},
	"0":   {},
}

// resolveURL 把 GitHub 原始 URL 包到代理前缀后。
// 代理前缀来源：环境变量 CFST_GITHUB_PROXY > 默认值 GitHubProxy；
// 设 "-" / "off" / "no" / "0" 表示禁用代理、保持原 URL。
// 输入必须以 https://github.com/ 开头；其它协议保持原样。
func resolveURL(raw string) string {
	if !strings.HasPrefix(raw, "https://github.com/") {
		return raw
	}
	prefix := GitHubProxy
	if v := strings.TrimSpace(os.Getenv("CFST_GITHUB_PROXY")); v != "" {
		lower := strings.ToLower(v)
		if _, off := proxyDisabledSentinels[lower]; off {
			return raw
		}
		prefix = v
	}
	// 兜底：用户给的前缀若不带尾斜杠则补上，避免 https://v4.gh-proxy.orggithub.com
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return prefix + raw
}

// LatestVersion 远端 version.txt 的内容（去除空白）
var LatestVersion string

// versionAPIURL 远端版本号地址（GitHub raw 直连地址）
const versionAPIURL = "https://github.com/masgzy/CloudflareST/raw/main/version.txt"

// CheckStatus 版本检查结果状态
//
//	检查失败、已是最新、发现新版本 三者必须严格区分，
//	绝不能把"检查失败"误报成"已是最新"（否则旧版本用户会被误导）
type CheckStatus int

const (
	CheckFailed    CheckStatus = iota // 检查失败（网络/代理等原因）
	CheckUpToDate                     // 已是最新版本
	CheckHasUpdate                    // 远端存在不同版本
)

// UpdateInfo 解析到的可更新信息
type UpdateInfo struct {
	Current   string
	Latest    string
	AssetName string // cfst_<os>_<arch>
	URL       string // 完整下载 URL
}

// CheckUpdate 检查远端 version.txt，单次请求最长 10s。
// 依次尝试：代理前缀 URL → GitHub 直连（任一成功即返回），
// 仅接受 HTTP 200 且内容形如 "vX.Y.Z..." 的响应，
// 避免把代理错误页当成版本号，也避免把网络失败误报成"已是最新"。
func CheckUpdate(currentVersion string) (CheckStatus, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	// 构造候选 URL：代理版在前，直连版兜底（CFST_GITHUB_PROXY=off 时仅直连）
	var candidates []string
	if proxied := resolveURL(versionAPIURL); proxied != versionAPIURL {
		candidates = append(candidates, proxied)
	}
	candidates = append(candidates, versionAPIURL)

	var lastErr error
	for _, url := range candidates {
		v, err := fetchVersion(client, url)
		if err != nil {
			lastErr = err
			continue
		}
		if v == strings.TrimSpace(currentVersion) {
			return CheckUpToDate, nil
		}
		LatestVersion = v
		return CheckHasUpdate, nil
	}
	return CheckFailed, lastErr
}

// fetchVersion 下载并校验远端版本号内容
func fetchVersion(client *http.Client, url string) (string, error) {
	res, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP 状态码 %d", res.StatusCode)
	}
	// 版本号极短，限制读取长度，防止把代理错误页/超长响应当成版本号
	body, err := io.ReadAll(io.LimitReader(res.Body, 64))
	if err != nil {
		return "", err
	}
	v := strings.TrimSpace(string(body))
	if !isValidVersion(v) {
		return "", fmt.Errorf("响应内容不是有效版本号: %q", v)
	}
	return v, nil
}

// isValidVersion 校验版本号格式：以 v 开头，仅含字母/数字/./-/_（如 v2.3.5-mod-3.1）
func isValidVersion(v string) bool {
	if len(v) < 2 || len(v) > 32 || v[0] != 'v' {
		return false
	}
	for i := 1; i < len(v); i++ {
		c := v[i]
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '.' || c == '-' || c == '_' {
			continue
		}
		return false
	}
	return true
}

// HasUpdate 当前是否检测到新版本
func HasUpdate() bool {
	return LatestVersion != ""
}

// PerformUpdate 执行更新流程：
//  1. 拼出下载 URL（基于当前 runtime 平台/架构）
//  2. 通过 GitHubProxy 代理下载到临时目录
//  3. 解压
//  4. POSIX 平台直接覆盖当前二进制；Windows 写 .new 并通过外部脚本完成替换
//  5. 成功返回 nil；中途任何错误返回详细原因
func PerformUpdate(ctx context.Context) error {
	asset, err := platform.AssetName()
	if err != nil {
		return err
	}
	rawURL, err := platform.DownloadURL()
	if err != nil {
		return err
	}
	downloadURL := resolveURL(rawURL)

	fmt.Printf("正在下载 %s ...\n", downloadURL)

	tmpDir, err := os.MkdirTemp("", "cfst-update-*")
	if err != nil {
		return fmt.Errorf("创建临时目录失败: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	archivePath := filepath.Join(tmpDir, asset+"."+platform.Ext(runtime.GOOS))
	if err := downloadFile(ctx, downloadURL, archivePath); err != nil {
		return fmt.Errorf("下载失败: %w", err)
	}
	fmt.Println("下载完成，正在解压...")

	extractDir := filepath.Join(tmpDir, "extract")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		return err
	}
	if err := extractArchive(archivePath, extractDir); err != nil {
		return fmt.Errorf("解压失败: %w", err)
	}

	newBinary, err := locateBinary(extractDir)
	if err != nil {
		return err
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("无法获取当前可执行文件路径: %w", err)
	}

	if platform.IsWindows() {
		return scheduleWindowsReplace(exe, newBinary)
	}
	return replaceBinary(exe, newBinary)
}

func downloadFile(ctx context.Context, url, dst string) error {
	cctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP 状态码 %d", resp.StatusCode)
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	bar := newProgressBar(resp.ContentLength, dst)
	defer bar.Finish()
	_, err = io.Copy(io.MultiWriter(f, bar), resp.Body)
	return err
}

func extractArchive(archivePath, destDir string) error {
	if strings.HasSuffix(archivePath, ".zip") {
		return extractZip(archivePath, destDir)
	}
	return extractTarGz(archivePath, destDir)
}

func extractTarGz(archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		// 防 Zip Slip：拒绝绝对路径与 ".." 跳出
		target, err := safeJoin(destDir, hdr.Name)
		if err != nil {
			return err
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode)&0o777)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		}
	}
}

func extractZip(archivePath, destDir string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		target, err := safeJoin(destDir, f.Name)
		if err != nil {
			return err
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode()&0o777)
		if err != nil {
			return err
		}
		in, err := f.Open()
		if err != nil {
			out.Close()
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			in.Close()
			out.Close()
			return err
		}
		in.Close()
		out.Close()
	}
	return nil
}

// safeJoin 拼接并阻止目录穿越
func safeJoin(parent, name string) (string, error) {
	clean := filepath.Clean("/" + name) // 强制以 / 开头，去掉 ..
	target := filepath.Join(parent, clean)
	if !strings.HasPrefix(target+string(os.PathSeparator), filepath.Clean(parent)+string(os.PathSeparator)) &&
		target != filepath.Clean(parent) {
		return "", fmt.Errorf("非法路径: %s", name)
	}
	return target, nil
}

// locateBinary 在解压目录中找 cfst 主二进制
func locateBinary(extractDir string) (string, error) {
	candidates := []string{"cfst", "cfst.exe"}
	var tried []string
	for _, name := range candidates {
		path, err := findFile(extractDir, name)
		if err == nil {
			return path, nil
		}
		tried = append(tried, err.Error())
	}
	return "", fmt.Errorf("在压缩包中未找到 cfst 主程序（尝试: %s）", strings.Join(tried, "; "))
}

func findFile(root, name string) (string, error) {
	var found string
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.EqualFold(info.Name(), name) {
			found = p
			return io.EOF
		}
		return nil
	})
	if err != nil && err != io.EOF {
		return "", err
	}
	if found == "" {
		return "", fmt.Errorf("未找到 %s", name)
	}
	return found, nil
}

// replaceBinary 直接覆盖当前二进制（POSIX）
func replaceBinary(current, new string) error {
	data, err := os.ReadFile(new)
	if err != nil {
		return err
	}
	// 写入临时文件后原子 rename
	tmp := current + ".new"
	if err := os.WriteFile(tmp, data, 0o755); err != nil {
		return err
	}
	if err := os.Rename(tmp, current); err != nil {
		os.Remove(tmp)
		return err
	}
	fmt.Printf("更新完成，新版本已写入 %s\n", current)
	return nil
}

// scheduleWindowsReplace 在 Windows 下：把新二进制放到 cfst.exe.new，
// 生成 update.bat 让用户确认后由它完成覆盖与重启。
// 流程：写 .new → 用户确认 → 启动 update.bat → 主程序退出 → 脚本覆盖并启动新进程
func scheduleWindowsReplace(current, new string) error {
	data, err := os.ReadFile(new)
	if err != nil {
		return err
	}
	stagePath := current + ".new"
	if err := os.WriteFile(stagePath, data, 0o755); err != nil {
		return err
	}

	batPath := filepath.Join(filepath.Dir(current), "cfst_update.bat")
	// 注意：批处理中 % 需转义为 %%，路径用引号包裹
	// 批处理脚本仅使用 ASCII 字符，避免 cmd.exe 默认 GBK 编码导致中文乱码
	script := fmt.Sprintf(`@echo off
setlocal
set "TARGET=%s"
set "STAGED=%s"
echo Updating CloudflareST, please wait...
:wait
ping -n 2 127.0.0.1 >nul
del /f /q "%%TARGET%%" 2>nul
move /y "%%STAGED%%" "%%TARGET%%" >nul
if errorlevel 1 goto wait
echo Update complete, restarting...
start "" "%%TARGET%%"
del /f /q "%%~f0"
endlocal
`, current, stagePath)
	if err := os.WriteFile(batPath, []byte(script), 0o644); err != nil {
		return err
	}
	fmt.Printf("已暂存新版本到 %s\n", stagePath)
	fmt.Println("按回车键关闭当前程序并完成更新（将自动重启）...")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n') // 读到 EOF（Ctrl+C/D）同样继续更新流程
	cmd := exec.Command("cmd", "/c", "start", "", batPath)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动更新脚本失败: %w", err)
	}
	os.Exit(0)
	return nil
}
