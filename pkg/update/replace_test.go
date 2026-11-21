// CloudflareST - Cloudflare CDN IP 延迟和速度测试工具
// Copyright (C) 2020-2026 masgzy <https://github.com/masgzy>
//
// 本程序使用 GPL-3.0 协议开源。
// 完整许可证文本见项目根目录 LICENSE 文件。

package update

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// S-2：O_EXCL 创建——预置符号链接必须被拒绝，且绝不跟随写入链接目标
func TestWriteNewFileExclusiveRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "cfst.new")
	target := filepath.Join(dir, "victim")
	if err := os.WriteFile(target, []byte("orig"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, p); err != nil {
		t.Fatal(err)
	}
	if err := writeNewFileExclusive(p, []byte("evil")); err == nil {
		t.Fatal("预置符号链接场景期望被拒绝，实际写入成功")
	}
	b, _ := os.ReadFile(target)
	if string(b) != "orig" {
		t.Fatalf("符号链接被跟随写入！目标文件内容: %q", b)
	}
}

// S-2：上次更新中断残留的常规文件应被清理后重试成功
func TestWriteNewFileExclusiveRetriesStale(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "cfst.new")
	if err := os.WriteFile(p, []byte("stale"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeNewFileExclusive(p, []byte("fresh")); err != nil {
		t.Fatalf("残留常规文件场景期望清理重试成功: %v", err)
	}
	b, _ := os.ReadFile(p)
	if string(b) != "fresh" {
		t.Fatalf("期望写入 fresh，实际 %q", b)
	}
}

// S-2：世界可写目录拒绝自动更新（本地提权面，宁可让用户手动更新）
func TestReplaceBinaryRejectsWorldWritableDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o777); err != nil {
		t.Skip("无法 chmod（可能是特殊文件系统）", err)
	}
	defer os.Chmod(dir, 0o700)
	cur := filepath.Join(dir, "cfst")
	newf := filepath.Join(dir, "new")
	if err := os.WriteFile(newf, []byte("bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	err := replaceBinary(cur, newf)
	if err == nil || !strings.Contains(err.Error(), "可写") {
		t.Fatalf("世界可写目录期望被拒绝，实际 err=%v", err)
	}
	if _, statErr := os.Stat(cur); !os.IsNotExist(statErr) {
		t.Fatal("拒绝路径不应产生新文件")
	}
}

// S-2：正常目录下 replaceBinary 原子替换成功
func TestReplaceBinarySuccess(t *testing.T) {
	dir := t.TempDir()
	cur := filepath.Join(dir, "cfst")
	newf := filepath.Join(dir, "new")
	if err := os.WriteFile(newf, []byte("newbin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := replaceBinary(cur, newf); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(cur)
	if string(b) != "newbin" {
		t.Fatalf("期望替换为 newbin，实际 %q", b)
	}
	if _, err := os.Stat(cur + ".new"); !os.IsNotExist(err) {
		t.Fatal("成功路径不应残留 .new 文件")
	}
}
