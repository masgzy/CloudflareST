package update

import "testing"

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name    string
		current string
		latest  string
		want    int
	}{
		{"完全相同", "v2.3.5-mod-3.5", "v2.3.5-mod-3.5", 0},
		{"远端 mod 号更新", "v2.3.5-mod-3.4", "v2.3.5-mod-3.5", -1},
		{"本地 mod 号更新（不应降级）", "v2.3.5-mod-3.5", "v2.3.5-mod-3.4", 1},
		{"远端主版本更新", "v2.3.5-mod-3.5", "v2.3.6", -1},
		{"远端大版本更新", "v2.3.5", "v3.0.0", -1},
		{"数字逐段比较而非字符串比较", "v2.3.9-mod-3.5", "v2.3.10", -1},
		{"本地纯版本 vs 远端 mod 版本", "v2.3.5", "v2.3.5-mod-3.5", -1},
		{"后缀文本不同但数字相同（保守提示更新）", "v2.3.5-beta", "v2.3.5-rc", -1},
		{"无法解析且不同（保守提示更新）", "v2.3.5-mod-3.5", "release", -1},
		{"无法解析但相同", "abc", "abc", 0},
		{"远端缺号（本地更完整）", "v2.3.5-mod-3.5", "v2.3", 1},
		{"空版本 vs 远端", "", "v2.3.5-mod-3.5", -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := compareVersions(tt.current, tt.latest); got != tt.want {
				t.Errorf("compareVersions(%q, %q) = %d, want %d", tt.current, tt.latest, got, tt.want)
			}
		})
	}
}
