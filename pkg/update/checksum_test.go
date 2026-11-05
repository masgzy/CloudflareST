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
	"os"
	"path/filepath"
	"testing"
)

const (
	hashABC   = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" // sha256("abc")
	hashEmpty = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" // sha256("")
)

func TestParseChecksumLine(t *testing.T) {
	cases := []struct {
		line string
		hash string
		name string
		ok   bool
	}{
		// 标准 sha256sum 输出（两个空格）
		{hashABC + "  cfst_linux_amd64.tar.gz", hashABC, "cfst_linux_amd64.tar.gz", true},
		// 单空格兼容
		{hashABC + " cfst_windows_amd64.zip", hashABC, "cfst_windows_amd64.zip", true},
		// 大写十六进制同样合法
		{"BA7816BF8F01CFEA414140DE5DAE2223B00361A396177A9CB410FF61F20015AD  upper.tar.gz",
			"BA7816BF8F01CFEA414140DE5DAE2223B00361A396177A9CB410FF61F20015AD", "upper.tar.gz", true},
		// 非法行：注释 / 哈希过短 / 非十六进制 / 缺文件名 / 空行
		{"# comment", "", "", false},
		{"short  a.tar.gz", "", "", false},
		{"zz" + hashABC[2:] + "  badhex.tar.gz", "", "", false},
		{hashABC, "", "", false},
		{"", "", "", false},
	}
	for i, c := range cases {
		hash, name, ok := parseChecksumLine(c.line)
		if ok != c.ok {
			t.Fatalf("用例 %d: parseChecksumLine(%q) ok = %v, 期望 %v", i, c.line, ok, c.ok)
		}
		if ok && (hash != c.hash || name != c.name) {
			t.Fatalf("用例 %d: got (%q, %q), 期望 (%q, %q)", i, hash, name, c.hash, c.name)
		}
	}
}

func TestLookupChecksum(t *testing.T) {
	h386 := "ec1c5f0f1e0e0e5e17e6ce3ec1c5f0f1e0e0e5e17e6ce3ec1c5f0f1e0e0e5e17"
	content := "# SHA256SUMS（CI 自动生成）\n" +
		h386 + "  cfst_linux_386.tar.gz\n" +
		hashABC + "  cfst_linux_amd64.tar.gz\n" +
		"\n" +
		"broken line without hash\n" +
		hashEmpty + "  cfst_darwin_arm64.zip\n"
	// 精确命中
	got, err := lookupChecksum(content, "cfst_linux_amd64.tar.gz")
	if err != nil || got != hashABC {
		t.Fatalf("精确命中失败: got %q, err %v", got, err)
	}
	// 子串/相近名不得误命中
	if _, err := lookupChecksum(content, "linux_amd64.tar.gz"); err == nil {
		t.Fatal("部分文件名不应命中")
	}
	if _, err := lookupChecksum(content, "cfst_linux_amd64.tar.gz.bak"); err == nil {
		t.Fatal("带后缀的相近名不应命中")
	}
	// 不存在的记录必须报错（而非返回空串继续流程）
	if _, err := lookupChecksum(content, "cfst_windows_arm64.zip"); err == nil {
		t.Fatal("缺失记录必须报错")
	}
	// 非法行不影响后续合法行
	if got, err := lookupChecksum(content, "cfst_darwin_arm64.zip"); err != nil || got != hashEmpty {
		t.Fatalf("非法行后的合法行解析失败: got %q, err %v", got, err)
	}
}

func TestFileSHA256(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "data.bin")
	if err := os.WriteFile(p, []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := fileSHA256(p)
	if err != nil {
		t.Fatal(err)
	}
	if got != hashABC {
		t.Fatalf("fileSHA256(abc) = %s, 期望 %s", got, hashABC)
	}
	// 空文件
	pe := filepath.Join(dir, "empty.bin")
	if err := os.WriteFile(pe, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if got, _ := fileSHA256(pe); got != hashEmpty {
		t.Fatalf("空文件摘要不符: %s", got)
	}
	// 不存在的文件必须报错
	if _, err := fileSHA256(filepath.Join(dir, "nope.bin")); err == nil {
		t.Fatal("不存在的文件应报错")
	}
}

func TestInsecureUpdateEnabled(t *testing.T) {
	cases := map[string]bool{
		"1": true, "true": true, "YES": true, "on": true,
		"0": false, "false": false, "off": false, "": false, "random": false,
	}
	for v, want := range cases {
		t.Setenv("CFST_UPDATE_INSECURE", v)
		if got := insecureUpdateEnabled(); got != want {
			t.Fatalf("CFST_UPDATE_INSECURE=%q: got %v, 期望 %v", v, got, want)
		}
	}
}

func TestProxyPrefix(t *testing.T) {
	// 默认环境（无 CFST_GITHUB_PROXY）应返回默认代理前缀
	t.Setenv("CFST_GITHUB_PROXY", "")
	if p := ProxyPrefix(); p == "" {
		t.Fatal("默认应返回代理前缀（默认代理开启）")
	}
	// 禁用代理时应返回空串
	t.Setenv("CFST_GITHUB_PROXY", "off")
	if p := ProxyPrefix(); p != "" {
		t.Fatalf("off 模式应返回空串, got %q", p)
	}
	// 自定义代理应原样返回（补尾斜杠）
	t.Setenv("CFST_GITHUB_PROXY", "https://my-proxy.example.com")
	if p := ProxyPrefix(); p != "https://my-proxy.example.com" {
		t.Fatalf("自定义代理前缀不符: %q", p)
	}
}
