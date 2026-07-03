# CLAUDE.md

CloudflareST 项目约定与知识库。

## 项目

- 仓库：`https://github.com/masgzy/CloudflareST`
- 协议：GPL-3.0（基于 [XIU2/CloudflareSpeedTest](https://github.com/XIU2/CloudflareSpeedTest) 二次开发）
- 主语言：Go（module `github.com/XIU2/CloudflareSpeedTest`）
- 入口：`main.go`；测速逻辑在 `task/`；通用工具在 `utils/`；自动更新：`pkg/update/`（`update.go` + `progress.go`）；平台/架构识别：`pkg/platform/`

## 版本号

- 格式：`vX.Y.Z-mod-N`（例：`v2.3.5-mod-2`）
- `version.txt` 永远指向当前最新发布版本
- 写 `Release.md` 时版本号必须向用户确认

## 自动更新（`-v`）

- 走 `releases/latest/download/cfst_系统_架构.<ext>` 直链，不调用 GitHub API
- 远端 `version.txt` 与 release 资产**统一通过 `pkg/update.GitHubProxy = "https://v4.gh-proxy.org/"` 代理**（按需改成自己的代理）
- 代理可通过环境变量 `CFST_GITHUB_PROXY` 覆盖：
  - `export CFST_GITHUB_PROXY=https://ghfast.top/`（自定义代理前缀，自动补尾 `/`）
  - `export CFST_GITHUB_PROXY=off` / `-` / `no` / `0`（禁用代理，直连 GitHub）
- 平台/架构由 `pkg/platform` 通过 `runtime.GOOS` / `runtime.GOARCH` 推断
  - Android / OpenHarmony 自动归一为 `linux`
  - ARM 32 位默认匹配 `armv7`；可通过环境变量 `CFST_UPDATE_ARCH=armv5|armv6|armv7` 覆盖
- Windows 走 `cfst.exe.new` + `cfst_update.bat` 外部脚本完成覆盖并自动重启
- 用户在提示下输入 `Y/n` 才执行；其它输入视为取消

## Cloudflare IP 段

- 已删除 `-cfips` 及其 `use/clear/update` 子命令
- 用户改用 `-f cf.txt`（IPv4）/ `-f cf6.txt`（IPv6）
- `cf.txt` / `cf6.txt` 由 `.github/workflows/release.yml` 在每次 build 时从
  `https://www.cloudflare.com/ips-v4/` 与 `https://www.cloudflare.com/ips-v6/`
  拉取并打包进所有 release 资源
- 自定义 IP 段并指定端口：`./cfst -ip 1.1.1.1:443,2.2.2.0/24:8443`

## 端口显示

- `utils.PingData` 新增 `Port int` 字段（0 表示使用全局 `TCPPort`）
- 端口来源：`task.IPPortMap[ip.String()]`，由 `task/ip.go` 在解析 IP 段时填充
- CSV 与终端表格都通过 `formatIPWithPort` 拼接 `IP:PORT`，无端口时只显示 `IP`
- 长字符串（IPv6 或带端口的 IPv4，>15 字符）会自动切换到更宽的列宽，避免界面错乱

## 平台下载表

下载链接统一格式：
`https://github.com/masgzy/CloudflareST/releases/latest/download/cfst_系统_架构.tar.gz` 或 `.zip`

| 系统        | 架构       | 位数   | 文件                                                                                                | 备注                                  |
|-------------|------------|--------|-----------------------------------------------------------------------------------------------------|---------------------------------------|
| macOS       | x86_64     | 64 位  | [cfst_darwin_amd64.zip](https://github.com/masgzy/CloudflareST/releases/latest/download/cfst_darwin_amd64.zip)         | 最低要求 macOS 11                     |
| macOS       | ARM v8     | 64 位  | [cfst_darwin_arm64.zip](https://github.com/masgzy/CloudflareST/releases/latest/download/cfst_darwin_arm64.zip)         | 最低要求 macOS 11                     |
| macOS       | x86_64     | 64 位  | [cfst_darwin_amd64_old.zip](https://github.com/masgzy/CloudflareST/releases/latest/download/cfst_darwin_amd64_old.zip) | 适用于 macOS 10 及旧版本              |
| macOS       | ARM v8     | 64 位  | [cfst_darwin_arm64_old.zip](https://github.com/masgzy/CloudflareST/releases/latest/download/cfst_darwin_arm64_old.zip) | 适用于 macOS 10 及旧版本              |
| Linux       | x86        | 32 位  | [cfst_linux_386.tar.gz](https://github.com/masgzy/CloudflareST/releases/latest/download/cfst_linux_386.tar.gz)         | 最低要求 Linux 内核 3.2（下同）       |
| Linux       | x86_64     | 64 位  | [cfst_linux_amd64.tar.gz](https://github.com/masgzy/CloudflareST/releases/latest/download/cfst_linux_amd64.tar.gz)     | ...                                   |
| Linux       | ARM v8     | 64 位  | [cfst_linux_arm64.tar.gz](https://github.com/masgzy/CloudflareST/releases/latest/download/cfst_linux_arm64.tar.gz)     | ARM v8 即 ARM 64 位 / AArch64         |
| Linux       | ARM v5     | 32 位  | [cfst_linux_armv5.tar.gz](https://github.com/masgzy/CloudflareST/releases/latest/download/cfst_linux_armv5.tar.gz)     | ...                                   |
| Linux       | ARM v6     | 32 位  | [cfst_linux_armv6.tar.gz](https://github.com/masgzy/CloudflareST/releases/latest/download/cfst_linux_armv6.tar.gz)     | ...                                   |
| Linux       | ARM v7     | 32 位  | [cfst_linux_armv7.tar.gz](https://github.com/masgzy/CloudflareST/releases/latest/download/cfst_linux_armv7.tar.gz)     | ...                                   |
| Linux       | Mips       | 32 位  | [cfst_linux_mips.tar.gz](https://github.com/masgzy/CloudflareST/releases/latest/download/cfst_linux_mips.tar.gz)       | ...                                   |
| Linux       | Mips       | 64 位  | [cfst_linux_mips64.tar.gz](https://github.com/masgzy/CloudflareST/releases/latest/download/cfst_linux_mips64.tar.gz)   | ...                                   |
| Linux       | Mipsle     | 32 位  | [cfst_linux_mipsle.tar.gz](https://github.com/masgzy/CloudflareST/releases/latest/download/cfst_linux_mipsle.tar.gz)   | ...                                   |
| Linux       | Mipsle     | 64 位  | [cfst_linux_mips64le.tar.gz](https://github.com/masgzy/CloudflareST/releases/latest/download/cfst_linux_mips64le.tar.gz) | ...                                   |
| Windows     | x86        | 32 位  | [cfst_windows_386.zip](https://github.com/masgzy/CloudflareST/releases/latest/download/cfst_windows_386.zip)           | 最低要求 Windows 10 + Server 2016     |
| Windows     | x86_64     | 64 位  | [cfst_windows_amd64.zip](https://github.com/masgzy/CloudflareST/releases/latest/download/cfst_windows_amd64.zip)       | 最低要求 Windows 10 + Server 2016     |
| Windows     | x86        | 32 位  | [cfst_windows_386_old.zip](https://github.com/masgzy/CloudflareST/releases/latest/download/cfst_windows_386_old.zip)   | 适用于 Windows 7/8 + Server 2008/2012 |
| Windows     | x86_64     | 64 位  | [cfst_windows_amd64_old.zip](https://github.com/masgzy/CloudflareST/releases/latest/download/cfst_windows_amd64_old.zip) | 适用于 Windows 7/8 + Server 2008/2012 |
| Windows     | ARM v8     | 64 位  | [cfst_windows_arm64.zip](https://github.com/masgzy/CloudflareST/releases/latest/download/cfst_windows_arm64.zip)       | 提供给 ARM 架构，别下错了            |

## 写 Release.md

由 skill `write-release-md` 负责：标题 `# vX.Y.Z-mod-N`、更新要点、上述下载表格。
新增章节插在文件开头，不覆盖历史。

## 常用命令

- 构建：`go build -ldflags="-s -w -X main.version=v2.3.5-mod-2" -o cfst .`
- 测速 Cloudflare：`./cfst -f cf.txt`
- 测速 IPv6：`./cfst -f cf6.txt`
- 检查并自动更新：`./cfst -v`
