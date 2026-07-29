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
	// 2枚のグラフ用に、指定/自動生成された出力ファイル名から
	// "_counts.png" / "_load.png" の2つのファイル名を作る。
	base := strings.TrimSuffix(outFile, ".png")
	countsFile := base + "_counts.png"
	loadFile := base + "_load.png"

	if err := outputCountsGraph(records, countsFile); err != nil {
		fmt.Println("failed to output counts graph:", err)
		os.Exit(1)
	}
	if err := outputLoadGraph(records, loadFile); err != nil {
		fmt.Println("failed to output load graph:", err)
		os.Exit(1)
	}

	fmt.Println("graphs saved to:", countsFile, "and", loadFile)
}

func loadRecords(path string) ([]metrics.Record, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return metrics.ReadCSV(f)
}

// toXYs は []float64（時刻と値）を plotter.XYs（X,Yの点列）に変換するヘルパー。
// outputCountsGraph / outputLoadGraph の両方から共通で使う。
func toXYs(times, ys []float64) plotter.XYs {
	pts := make(plotter.XYs, len(times))
	for i := range times {
		pts[i].X = times[i]
		pts[i].Y = ys[i]
	}
	return pts
}

// outputCountsGraph は、件数ベースの指標（Rate, BgLoad, OK, Fail）だけを
// 1つのグラフにまとめて filename に書き出す。
//
// AvgLoad（0.0〜1.0の比率）と同じ軸に載せると、スケールの違いから
// AvgLoadの変動がグラフの下側に埋もれて見えにくくなるため、
// 件数系と比率系でグラフ自体を分けている。
func outputCountsGraph(records []metrics.Record, filename string) error {
	p := plot.New()

	p.Title.Text = "Backpressure Demo: Request Counts Over Time"
	p.X.Label.Text = "Time (s)"
	p.Y.Label.Text = "Count"

	n := len(records)
	times := make([]float64, n)
	rates := make([]float64, n)
	bgLoads := make([]float64, n)
	okCounts := make([]float64, n)
	failCounts := make([]float64, n)

	for i, r := range records {
		times[i] = r.TimeS
		rates[i] = r.Rate
		bgLoads[i] = r.BgLoad
		okCounts[i] = r.OK
		failCounts[i] = r.Fail
	}

	err := plotutil.AddLinePoints(p,
		"Rate", toXYs(times, rates),
		"BgLoad", toXYs(times, bgLoads),
		"OK", toXYs(times, okCounts),
		"Fail", toXYs(times, failCounts),
	)
	if err != nil {
		return err
	}

	return p.Save(8*vg.Inch, 4*vg.Inch, filename)
}

// outputLoadGraph は、比率ベースの指標（AvgLoad, 0.0〜1.0）だけを
// 1つのグラフにまとめて filename に書き出す。
func outputLoadGraph(records []metrics.Record, filename string) error {
	p := plot.New()

	p.Title.Text = "Backpressure Demo: Average Load Over Time"
	p.X.Label.Text = "Time (s)"
	p.Y.Label.Text = "Load (0.0-1.0)"

	n := len(records)
	times := make([]float64, n)
	avgLoads := make([]float64, n)

	for i, r := range records {
		times[i] = r.TimeS
		avgLoads[i] = r.AvgLoad
	}

	err := plotutil.AddLinePoints(p,
		"AvgLoad", toXYs(times, avgLoads),
	)
	if err != nil {
		return err
	}

	return p.Save(8*vg.Inch, 4*vg.Inch, filename)
}