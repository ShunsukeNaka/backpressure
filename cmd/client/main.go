package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/plotutil"
	"gonum.org/v1/plot/vg"
)

func main() {
	naive := flag.Bool("naive", false, "AIMDを使わず固定レートで送り続ける（比較用）")
	fixedRate := flag.Float64("fixed-rate", 15.0, "-naive 使用時の固定リクエストレート")
	ticks := flag.Int("ticks", 40, "実行する秒数")
	target := flag.String("target", "http://localhost:8082/", "呼び出し先URL（デフォルトはMiddle経由。Leafに直接投げるなら http://localhost:8081/ を指定）")
	flag.Parse()

	rate := 5.0
	const minRate = 1.0

	client := &http.Client{Timeout: 2 * time.Second}

	fmt.Println("mode:", modeLabel(*naive))
	fmt.Println("time_s\trate\tavg_load\tok\tfail")

	// --- 各tickの値を溜めておくスライス（グラフ描画用） ---
	times := make([]float64, *ticks)
	rates := make([]float64, *ticks)
	avgLoads := make([]float64, *ticks)
	okCounts := make([]float64, *ticks)
	failCounts := make([]float64, *ticks)

	for tick := 0; tick < *ticks; tick++ {
		sendRate := rate
		if *naive {
			sendRate = *fixedRate
		}

		n := int(sendRate)
		if n < 1 {
			n = 1
		}

		var wg sync.WaitGroup
		var mu sync.Mutex
		var loadSum float64
		var loadCount int
		var okCount, failCount int

		for i := 0; i < n; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				resp, err := client.Get(*target)
				if err != nil {
					mu.Lock()
					failCount++
					mu.Unlock()
					return
				}
				defer resp.Body.Close()
				io.Copy(io.Discard, resp.Body)

				loadStr := resp.Header.Get("X-Load")
				load, parseErr := strconv.ParseFloat(loadStr, 64)

				mu.Lock()
				if resp.StatusCode == http.StatusOK {
					okCount++
				} else {
					failCount++
				}
				if parseErr == nil {
					loadSum += load
					loadCount++
				}
				mu.Unlock()
			}()
		}
		wg.Wait()

		avgLoad := 0.0
		if loadCount > 0 {
			avgLoad = loadSum / float64(loadCount)
		}

		// --- ここが分散バックプレッシャー伝播のコア ---
		if !*naive {
			switch {
			case avgLoad > 0.8:
				rate = rate * 0.5 // Multiplicative Decrease
				if rate < minRate {
					rate = minRate
				}
			case avgLoad < 0.3:
				rate = rate + 1 // Additive Increase
			}
		}

		fmt.Printf("%d\t%.1f\t%.2f\t%d\t%d\n", tick, sendRate, avgLoad, okCount, failCount)

		// --- この tick の値を記録（グラフ描画用） ---
		times[tick] = float64(tick)
		rates[tick] = sendRate
		avgLoads[tick] = avgLoad
		okCounts[tick] = float64(okCount)
		failCounts[tick] = float64(failCount)

		time.Sleep(1 * time.Second)
	}

	// --- 設定内容をファイル名に組み込む ---
	filename := buildFilename(*naive, *fixedRate)

	if err := outputGraph(times, rates, avgLoads, okCounts, failCounts, filename); err != nil {
		fmt.Println("failed to output graph:", err)
	} else {
		fmt.Println("graph saved to:", filename)
	}
}

func modeLabel(naive bool) string {
	if naive {
		return "naive (fixed rate, no backpressure)"
	}
	return "AIMD (backpressure-aware)"
}

// buildFilename は、実行時の設定（AIMDか固定か、固定ならそのレート）と
// プロトコルバージョンを組み込んだファイル名を組み立てる。
// 例: request_rate_aimd_v1.png / request_rate_naive_rate20_v1.png
func buildFilename(naive bool, fixedRate float64) string {
	mode := "aimd"
	if naive {
		mode = fmt.Sprintf("naive_rate%.0f", fixedRate)
	}
	return fmt.Sprintf("request_rate_%s.png", mode)
}

// outputGraph は time_s を横軸に、rate / avg_load / ok / fail の
// 4項目を1つの折れ線グラフとして filename に書き出す。
func outputGraph(times, rates, avgLoads, okCounts, failCounts []float64, filename string) error {
	p := plot.New()

	p.Title.Text = "Backpressure Demo: Metrics Over Time"
	p.X.Label.Text = "Time (s)"
	p.Y.Label.Text = "Value"

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
		"OK", toXYs(okCounts),
		"Fail", toXYs(failCounts),
	)
	if err != nil {
		return err
	}

	return p.Save(8*vg.Inch, 4*vg.Inch, filename)
}