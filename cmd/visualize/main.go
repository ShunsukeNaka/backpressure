// cmd/visualize は、cmd/client が出力したCSVを読み込んで、
// 折れ線グラフのPNGとして書き出すだけの、可視化専用コマンド。
//
// 計測（cmd/client）と可視化（このコマンド）を分けることで、
// gonum/plot への依存をこちらに閉じ込め、計測結果（CSV）自体は
// 後から何度でも別の形式で可視化し直せるようにしている。
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/plotutil"
	"gonum.org/v1/plot/vg"

	"backpressure-demo/internal/metrics"
)

func main() {
	in := flag.String("in", "", "入力CSVファイル（cmd/client が出力したもの）")
	out := flag.String("out", "", "出力PNGファイル。省略時は入力ファイル名から自動生成 (例: foo.csv -> foo.png)")
	flag.Parse()

	if *in == "" {
		fmt.Println("使い方: go run ./cmd/visualize -in=request_rate_aimd.csv")
		os.Exit(1)
	}

	records, err := loadRecords(*in)
	if err != nil {
		fmt.Println("failed to load csv:", err)
		os.Exit(1)
	}

	outFile := *out
	if outFile == "" {
		outFile = strings.TrimSuffix(*in, ".csv") + ".png"
	}

	if err := outputGraph(records, outFile); err != nil {
		fmt.Println("failed to output graph:", err)
		os.Exit(1)
	}

	fmt.Println("graph saved to:", outFile)
}

func loadRecords(path string) ([]metrics.Record, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return metrics.ReadCSV(f)
}

// outputGraph は records を time_s を横軸にした折れ線グラフとして filename に書き出す。
func outputGraph(records []metrics.Record, filename string) error {
	p := plot.New()

	p.Title.Text = "Backpressure Demo: Metrics Over Time"
	p.X.Label.Text = "Time (s)"
	p.Y.Label.Text = "Value"

	n := len(records)
	times := make([]float64, n)
	rates := make([]float64, n)
	avgLoads := make([]float64, n)
	bgLoads := make([]float64, n)
	okCounts := make([]float64, n)
	failCounts := make([]float64, n)

	for i, r := range records {
		times[i] = r.TimeS
		rates[i] = r.Rate
		avgLoads[i] = r.AvgLoad
		bgLoads[i] = r.BgLoad
		okCounts[i] = r.OK
		failCounts[i] = r.Fail
	}

	// []float64 を plotter.XYs（X,Yの点列）に変換するヘルパー
	toXYs := func(ys []float64) plotter.XYs {
		pts := make(plotter.XYs, len(times))
		for i := range times {
			pts[i].X = times[i]
			pts[i].Y = ys[i]
		}
		return pts
	}

	err := plotutil.AddLinePoints(p,
		"Rate", toXYs(rates),
		"AvgLoad", toXYs(avgLoads),
		"BgLoad", toXYs(bgLoads),
		"OK", toXYs(okCounts),
		"Fail", toXYs(failCounts),
	)
	if err != nil {
		return err
	}

	return p.Save(8*vg.Inch, 4*vg.Inch, filename)
}
