package utils

import "testing"

func TestDisplayWidth(t *testing.T) {
        cases := []struct {
                in   string
                want int
        }{
                {"", 0},
                {"104.16.0.0", 10},
                {"abc", 3},
                {"IP 地址", 7},              // "IP " = 3，"地址" = 4
                {"下载速度(MB/s)", 14},        // "下载速度" = 8 + "(MB/s)" = 6
                {"[2606:4700::]:443", 17}, // 纯 ASCII
                {"已发送", 6},
                {"地区码", 6},
                {"全角１２３", 10}, // 2 个汉字 + 3 个全角数字，共 5 个宽字符
        }
        for _, c := range cases {
                if got := displayWidth(c.in); got != c.want {
                        t.Errorf("displayWidth(%q) = %d, want %d", c.in, got, c.want)
                }
        }
}

func TestPadDisplay(t *testing.T) {
        cases := []struct {
                in       string
                width    int
                wantCols int // 期望补齐后的显示宽度
        }{
                {"abc", 6, 6},
                {"地址", 6, 6},     // 宽字符按 2 列补齐
                {"abcdef", 3, 6}, // 超宽时原样返回
                {"", 4, 4},
                {"IP", 0, 2}, // 宽度为 0 时原样返回
        }
        for _, c := range cases {
                got := padDisplay(c.in, c.width)
                if w := displayWidth(got); w != c.wantCols {
                        t.Errorf("padDisplay(%q, %d) 显示宽度 = %d, want %d", c.in, c.width, w, c.wantCols)
                }
        }
        // 超宽时必须原样返回，不能截断
        if got := padDisplay("abcdef", 3); got != "abcdef" {
                t.Errorf("padDisplay 超宽时应原样返回，实际 %q", got)
        }
}

func BenchmarkDisplayWidthPad(b *testing.B) {
        s := "2606:4700::/48 采样地址"
        b.ResetTimer()
        for i := 0; i < b.N; i++ {
                _ = padDisplay(s, 20)
        }
}
