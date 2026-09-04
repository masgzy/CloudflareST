package utils

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// makeData 构造一条测速结果数据
func makeData(ipStr string, sended, received int, delay time.Duration, speed float64) CloudflareIPData {
	return CloudflareIPData{
		PingData: &PingData{
			IP:       &net.IPAddr{IP: net.ParseIP(ipStr)},
			Sended:   sended,
			Received: received,
			Delay:    delay,
		},
		DownloadSpeed: speed,
	}
}

func TestSortBySpeed(t *testing.T) {
	old := UseZScore
	defer func() { UseZScore = old }()
	UseZScore = false

	data := DownloadSpeedSet{
		makeData("1.1.1.1", 4, 4, 50*time.Millisecond, 1*1024*1024),
		makeData("2.2.2.2", 4, 4, 50*time.Millisecond, 3*1024*1024),
		makeData("3.3.3.3", 4, 4, 50*time.Millisecond, 2*1024*1024),
	}
	data.Sort()
	speeds := []float64{data[0].DownloadSpeed, data[1].DownloadSpeed, data[2].DownloadSpeed}
	if speeds[0] <= speeds[1] || speeds[1] <= speeds[2] {
		t.Errorf("默认应按速度降序，实际 %v", speeds)
	}
}

// TestSortZScore 综合排序：速度最高但丢包严重的 A 应排在零丢包、速度略低的 B 之后
func TestSortZScore(t *testing.T) {
	old := UseZScore
	defer func() { UseZScore = old }()
	UseZScore = true

	data := DownloadSpeedSet{
		makeData("1.1.1.1", 1000, 500, 50*time.Millisecond, 10*1024*1024), // A：速度最高，丢包 50%
		makeData("2.2.2.2", 1000, 1000, 50*time.Millisecond, 9*1024*1024), // B：速度略低，零丢包
		makeData("3.3.3.3", 1000, 1000, 50*time.Millisecond, 1*1024*1024), // C：速度最低
	}
	data.Sort()
	if data[0].IP.String() != "2.2.2.2" {
		t.Errorf("综合排序应把零丢包的 2.2.2.2 排第一，实际第一是 %s（速度 %.1f MB/s，丢包 %.2f）",
			data[0].IP.String(), data[0].DownloadSpeed/1024/1024, data[0].getLossRate())
	}
	if data[1].IP.String() != "1.1.1.1" {
		t.Errorf("综合排序第二应为 1.1.1.1，实际 %s", data[1].IP.String())
	}
}

func TestExportCsvBOM(t *testing.T) {
	old := Output
	defer func() { Output = old }()

	Output = filepath.Join(t.TempDir(), "result.csv")
	data := DownloadSpeedSet{
		makeData("104.16.1.1", 4, 4, 50*time.Millisecond, 2.5*1024*1024),
	}
	ExportCsv(data)

	raw, err := os.ReadFile(Output)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(raw, []byte{0xEF, 0xBB, 0xBF}) {
		t.Error("CSV 文件缺少 UTF-8 BOM，Excel 打开中文表头会乱码")
	}
	if !bytes.Contains(raw, []byte("IP 地址")) {
		t.Error("CSV 缺少中文表头")
	}
	if !bytes.Contains(raw, []byte("104.16.1.1")) {
		t.Error("CSV 缺少数据行")
	}
}

func TestExportCsvNoOutput(t *testing.T) {
	old := Output
	defer func() { Output = old }()

	Output = "" // -o "" 不写文件，不应报错
	ExportCsv(DownloadSpeedSet{makeData("1.1.1.1", 4, 4, time.Millisecond, 1)})
}

func TestPrintNegativePrintNum(t *testing.T) {
	old := PrintNum
	defer func() { PrintNum = old }()

	PrintNum = -1 // 负数不应 panic，也不应打印
	data := DownloadSpeedSet{makeData("1.1.1.1", 4, 4, time.Millisecond, 1)}
	data.Print() // 仅验证不 panic
}

func BenchmarkZScoreSort100(b *testing.B) {
	old := UseZScore
	defer func() { UseZScore = old }()
	UseZScore = true

	data := make(DownloadSpeedSet, 0, 100)
	for i := 0; i < 100; i++ {
		data = append(data, makeData(
			"104.16.1.1",
			4, 4-(i%3),
			time.Duration(50+i)*time.Millisecond,
			float64(1+i%10)*1024*1024,
		))
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		data.Sort()
	}
}
