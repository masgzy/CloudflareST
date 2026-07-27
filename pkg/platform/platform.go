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

// Package platform 提供系统/架构识别能力，用于自动更新。
//
// 设计目标：
//   - 把当前运行的操作系统/架构映射到 release 压缩包文件名：
//     cfst_<os>_<arch>.{tar.gz|zip} 或 cfst_<os>_<arch>_old.{tar.gz|zip}
//   - Android 与 OpenHarmony 自动归一为 linux（这些系统的 runtime.GOOS 即 "linux"）
//   - ARM 32 位按 GOARM 进一步区分 armv5/6/7
package platform

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

// AssetName 返回当前平台对应的 release 资产文件名（不含扩展名前缀）。
// 例子：cfst_linux_amd64 / cfst_linux_armv7 / cfst_darwin_amd64 / cfst_windows_amd64
func AssetName() (string, error) {
	osName, err := mapOS(runtime.GOOS)
	if err != nil {
		return "", err
	}
	archName, err := mapArch(runtime.GOARCH, runtime.GOOS)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("cfst_%s_%s", osName, archName), nil
}

// AssetNameWithOld 用于下载 *_old.* 老系统资产。
func AssetNameWithOld() (string, error) {
	base, err := AssetName()
	if err != nil {
		return "", err
	}
	return base + "_old", nil
}

// Ext 根据操作系统返回压缩包扩展名：windows=zip，其它=tar.gz
func Ext(goos string) string {
	if goos == "windows" {
		return "zip"
	}
	return "tar.gz"
}

// mapOS 把 runtime.GOOS 映射为 release 文件名里的 OS 段。
// Android/OpenHarmony 都被归一为 linux。
func mapOS(goos string) (string, error) {
	switch goos {
	case "linux":
		// Android 与 OpenHarmony 的 runtime.GOOS 同样是 "linux"，无需额外判断。
		return "linux", nil
	case "darwin":
		return "darwin", nil
	case "windows":
		return "windows", nil
	default:
		return "", fmt.Errorf("不支持的操作系统: %s", goos)
	}
}

// mapArch 把 runtime.GOARCH 映射为 release 文件名里的 arch 段。
// ARM 32 位具体是 v5/v6/v7 在运行时无法直接获取 GOARM，统一默认为 armv7。
// 如需在 armv5/armv6 设备上更新，可设置环境变量 CFST_UPDATE_ARCH=armv5 或 armv6。
func mapArch(goarch, goos string) (string, error) {
	switch goarch {
	case "amd64":
		return "amd64", nil
	case "386":
		return "386", nil
	case "arm64":
		return "arm64", nil
	case "arm":
		if goos != "linux" {
			return "", fmt.Errorf("arm 架构仅在 linux 下区分 v5/v6/v7 (当前: %s)", goos)
		}
		override := strings.ToLower(strings.TrimSpace(os.Getenv("CFST_UPDATE_ARCH")))
		switch override {
		case "armv5", "armv6", "armv7":
			return override, nil
		}
		return "armv7", nil
	case "mips":
		return "mips", nil
	case "mips64":
		return "mips64", nil
	case "mipsle":
		return "mipsle", nil
	case "mips64le":
		return "mips64le", nil
	default:
		return "", fmt.Errorf("不支持的 CPU 架构: %s", goarch)
	}
}

// DownloadURL 直接拼出 release/latest/download 的稳定 URL，不走 GitHub API。
// 形式：https://github.com/masgzy/CloudflareST/releases/latest/download/cfst_<os>_<arch>.<ext>
func DownloadURL() (string, error) {
	name, err := AssetName()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("https://github.com/masgzy/CloudflareST/releases/latest/download/%s.%s",
		name, Ext(runtime.GOOS)), nil
}

// IsWindows 便捷判断：是否在 Windows 上运行（更新逻辑要写 .new 文件 + 外部脚本）。
func IsWindows() bool {
	return runtime.GOOS == "windows"
}

// CurrentOSArch 调试辅助：返回 runtime 实际值，便于日志输出。
func CurrentOSArch() string {
	parts := []string{runtime.GOOS, runtime.GOARCH}
	return strings.Join(parts, "/")
}
