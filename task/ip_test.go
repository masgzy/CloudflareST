package task

import (
	"os"
	"path/filepath"
	"testing"
)

// resetIPGlobals 重置 IP 解析相关的包级全局变量，避免测试间相互污染
func resetIPGlobals(t *testing.T) {
	t.Helper()
	oldIPText, oldIPFile, oldTestAll := IPText, IPFile, TestAll
	oldPortMap, oldPortUser := IPPortMap, IPPortFromUser
	IPPortMap = make(map[string]int)
	IPPortFromUser = make(map[string]bool)
	t.Cleanup(func() {
		IPText, IPFile, TestAll = oldIPText, oldIPFile, oldTestAll
		IPPortMap, IPPortFromUser = oldPortMap, oldPortUser
	})
	IPText, IPFile, TestAll = "", defaultInputFile, false
	IPPortMap = make(map[string]int)
	IPPortFromUser = make(map[string]bool)
}

func TestExtractPort(t *testing.T) {
	cases := []struct {
		in       string
		wantIP   string
		wantPort int
	}{
		{"1.1.1.1:443", "1.1.1.1", 443},
		{"1.1.1.0/24:443", "1.1.1.0/24", 443},
		{"[::1]:443", "::1", 443},
		{"[2606:4700::/32]:443", "2606:4700::/32", 443},
		{"::1", "::1", 0},
		{"[::1]", "::1", 0},
		{"2606:4700::/32", "2606:4700::/32", 0},
		{"1.1.1.1", "1.1.1.1", 0},
		{"1.1.1.1:abc", "1.1.1.1:abc", 0}, // 端口非法时整体原样返回，交给 ParseCIDR 报错
	}
	for _, c := range cases {
		gotIP, gotPort := extractPort(c.in)
		if gotIP != c.wantIP || gotPort != c.wantPort {
			t.Errorf("extractPort(%q) = (%q, %d), want (%q, %d)", c.in, gotIP, gotPort, c.wantIP, c.wantPort)
		}
	}
}

func TestParseSourceEntry(t *testing.T) {
	cases := []struct {
		in      string
		wantIP  string
		wantCnt int
		wantErr bool
	}{
		{"1.1.1.0/24", "1.1.1.0/24", 0, false},
		{"2606:4700::/48=1000", "2606:4700::/48", 1000, false},
		{"1.1.1.0/24 = 500", "1.1.1.0/24", 500, false},
		{"1.1.1.1:443=10", "1.1.1.1:443", 10, false},
		{"[2606:4700::/48]:443=200", "[2606:4700::/48]:443", 200, false},
		{"1.1.1.0/24=0", "", 0, true},
		{"1.1.1.0/24=abc", "", 0, true},
		{"1.1.1.0/24=-5", "", 0, true},
		{"=1000", "", 0, true},
	}
	for _, c := range cases {
		entry, err := parseSourceEntry(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseSourceEntry(%q) 期望报错，实际无错", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseSourceEntry(%q) 意外报错: %v", c.in, err)
			continue
		}
		if entry.ipPart != c.wantIP || entry.count != c.wantCnt {
			t.Errorf("parseSourceEntry(%q) = (%q, %d), want (%q, %d)", c.in, entry.ipPart, entry.count, c.wantIP, c.wantCnt)
		}
	}
}

func TestCleanIPSourceLine(t *testing.T) {
	cases := []struct {
		in, out string
	}{
		{"  1.1.1.0/24  ", "1.1.1.0/24"},
		{"", ""},
		{"   ", ""},
		{"# 注释", ""},
		{"// 注释", ""},
		{"#1.1.1.0/24", ""},
		{"1.1.1.0/#24", "1.1.1.0/#24"}, // 仅行首才算注释
	}
	for _, c := range cases {
		if got := cleanIPSourceLine(c.in); got != c.out {
			t.Errorf("cleanIPSourceLine(%q) = %q, want %q", c.in, got, c.out)
		}
	}
}

func TestCollectIPSourcesFromParam(t *testing.T) {
	resetIPGlobals(t)
	IPText = "1.1.1.0/24, 1.1.1.0/24 ,2.2.2.0/24,,#comment,// comment"
	entries, err := collectIPSources()
	if err != nil {
		t.Fatalf("意外报错: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("期望 2 个条目（重复与注释被过滤），实际 %d 个: %+v", len(entries), entries)
	}
	if entries[0].ipPart != "1.1.1.0/24" || entries[1].ipPart != "2.2.2.0/24" {
		t.Errorf("条目顺序/内容不符: %+v", entries)
	}
}

func TestCollectIPSourcesDedupWithCount(t *testing.T) {
	resetIPGlobals(t)
	IPText = "1.1.1.0/24=10,1.1.1.0/24=10,2.2.2.0/24=20"
	entries, err := collectIPSources()
	if err != nil {
		t.Fatalf("意外报错: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("期望 2 个条目，实际 %d 个: %+v", len(entries), entries)
	}
	if entries[0].count != 10 || entries[1].count != 20 {
		t.Errorf("采样数量不符: %+v", entries)
	}
}

func TestCollectIPSourcesFromFile(t *testing.T) {
	resetIPGlobals(t)
	content := "# 顶部注释\n\n// 另一种注释\n  3.3.3.0/24  \n3.3.3.0/24\n4.4.4.0/24=100\n"
	file := filepath.Join(t.TempDir(), "ip.txt")
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	IPFile = file
	entries, err := collectIPSources()
	if err != nil {
		t.Fatalf("意外报错: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("期望 2 个条目（注释行/空行/重复被过滤），实际 %d 个: %+v", len(entries), entries)
	}
	if entries[0].ipPart != "3.3.3.0/24" || entries[1].ipPart != "4.4.4.0/24" || entries[1].count != 100 {
		t.Errorf("条目内容不符: %+v", entries)
	}
}

func TestCollectIPSourcesParamPriority(t *testing.T) {
	resetIPGlobals(t)
	file := filepath.Join(t.TempDir(), "ip.txt")
	if err := os.WriteFile(file, []byte("5.5.5.0/24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	IPFile = file
	IPText = "1.1.1.0/24"
	entries, err := collectIPSources()
	if err != nil {
		t.Fatalf("意外报错: %v", err)
	}
	if len(entries) != 1 || entries[0].ipPart != "1.1.1.0/24" {
		t.Errorf("[-ip] 应优先于 [-f]: %+v", entries)
	}
}

func TestCollectIPSourcesErrors(t *testing.T) {
	resetIPGlobals(t)
	IPFile = filepath.Join(t.TempDir(), "not_exist.txt")
	if _, err := collectIPSources(); err == nil {
		t.Error("文件不存在时应报错")
	}

	resetIPGlobals(t)
	IPText = "1.1.1.0/24=abc"
	if _, err := collectIPSources(); err == nil {
		t.Error("采样数量非法时应报错")
	}
}

// checkSampled 校验采样结果：数量正确、全部落在网段内、互不重复
func checkSampled(t *testing.T, ranges *IPRanges, want int) {
	t.Helper()
	if len(ranges.ips) != want {
		t.Fatalf("期望采样 %d 个 IP，实际 %d 个", want, len(ranges.ips))
	}
	seen := make(map[string]struct{}, want)
	for _, ip := range ranges.ips {
		if !ranges.ipNet.Contains(ip.IP) {
			t.Fatalf("采样 IP %s 超出网段 %s", ip.String(), ranges.ipNet.String())
		}
		key := ip.IP.String()
		if _, dup := seen[key]; dup {
			t.Fatalf("采样 IP 重复: %s", key)
		}
		seen[key] = struct{}{}
	}
}

func TestChooseIPv4Counted(t *testing.T) {
	resetIPGlobals(t)

	ranges := newIPRanges()
	ranges.parseCIDR("1.1.1.0/24")
	ranges.chooseIPv4Counted(50)
	checkSampled(t, ranges, 50)

	ranges = newIPRanges()
	ranges.parseCIDR("1.1.1.0/30")
	ranges.chooseIPv4Counted(100) // 数量超过网段地址总数，钳制为 4
	checkSampled(t, ranges, 4)

	ranges = newIPRanges()
	ranges.parseCIDR("104.16.0.0/12")
	ranges.chooseIPv4Counted(4096)
	checkSampled(t, ranges, 4096)

	ranges = newIPRanges()
	ranges.parseCIDR("1.1.1.1/32") // 单个 IP 指定数量，应只返回自身
	ranges.chooseIPv4Counted(10)
	checkSampled(t, ranges, 1)
}

func TestChooseIPv4CountedPort(t *testing.T) {
	resetIPGlobals(t)

	ranges := newIPRanges()
	ranges.parseCIDR("1.1.1.0/24:8443")
	ranges.chooseIPv4Counted(5)
	checkSampled(t, ranges, 5)
	if len(IPPortMap) != 5 {
		t.Fatalf("期望 5 个端口映射，实际 %d 个", len(IPPortMap))
	}
	for ip, port := range IPPortMap {
		if port != 8443 {
			t.Errorf("IP %s 端口映射为 %d，期望 8443", ip, port)
		}
	}
}

func TestChooseIPv6Counted(t *testing.T) {
	resetIPGlobals(t)

	ranges := newIPRanges()
	ranges.parseCIDR("2606:4700::/48")
	ranges.chooseIPv6Counted(100)
	checkSampled(t, ranges, 100)

	ranges = newIPRanges()
	ranges.parseCIDR("2606:4700::/126")
	ranges.chooseIPv6Counted(10) // 钳制为 4
	checkSampled(t, ranges, 4)

	ranges = newIPRanges()
	ranges.parseCIDR("::1/128")
	ranges.chooseIPv6Counted(5) // 单个 IP
	checkSampled(t, ranges, 1)
}

// 空来源（如文件全是注释）时 collectIPSources 返回 0 条，
// loadIPRanges 会 log.Fatal 提示（os.Exit 无法进程内断言，由集成实测覆盖）
func TestCollectIPSourcesAllComments(t *testing.T) {
	resetIPGlobals(t)
	file := filepath.Join(t.TempDir(), "empty.txt")
	if err := os.WriteFile(file, []byte("# 全是注释\n\n// 也是注释\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	IPFile = file
	entries, err := collectIPSources()
	if err != nil {
		t.Fatalf("纯注释文件应收集到 0 条而非报错: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("期望 0 条，实际 %d 条", len(entries))
	}
}

func BenchmarkChooseIPv4Counted(b *testing.B) {
	ranges := newIPRanges()
	ranges.parseCIDR("104.16.0.0/12")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ranges.ips = ranges.ips[:0]
		ranges.chooseIPv4Counted(8192)
	}
}

func BenchmarkChooseIPv6Counted(b *testing.B) {
	ranges := newIPRanges()
	ranges.parseCIDR("2606:4700::/48")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ranges.ips = ranges.ips[:0]
		ranges.chooseIPv6Counted(1000)
	}
}

func BenchmarkChooseIPv4Default(b *testing.B) {
	ranges := newIPRanges()
	ranges.parseCIDR("104.16.0.0/16")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ranges.ips = ranges.ips[:0]
		ranges.chooseIPv4()
	}
}
