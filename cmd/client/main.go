package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"backpressure-demo/internal/metrics"
	"backpressure-demo/pkg/backpressure"
)

func main() {
	naive := flag.Bool("naive", false, "AIMDを使わず固定レートで送り続ける（比較用）")
	fixedRate := flag.Float64("fixed-rate", 15.0, "-naive 使用時の固定リクエストレート")
	ticks := flag.Int("ticks", 40, "実行する秒数")
	target := flag.String("target", "http://localhost:8082/", "呼び出し先URL（デフォルトはMiddle経由。Leafに直接投げるなら http://localhost:8081/ を指定）")
	leafStats := flag.String("leaf-stats", "http://localhost:8081/stats", "Leafの背景負荷を取得するstatsエンドポイント")
	flag.Parse()

	// レート調整の判断ロジックは pkg/backpressure の AIMDController に委譲する。
	ctrl := backpressure.NewAIMDController(5.0)

	client := &http.Client{Timeout: 2 * time.Second}

	fmt.Println("mode:", modeLabel(*naive))
	fmt.Println("time_s\trate\tavg_load\tbg_load\tok\tfail")

	// --- 計測結果を溜めておくスライス（CSV出力用） ---
	records := make([]metrics.Record, 0, *ticks)

	for tick := 0; tick < *ticks; tick++ {
		sendRate := ctrl.Rate
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

				load, parseErr := backpressure.ParseLoad(resp)

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

		// --- 背景負荷の取得（可視化用。AIMDの判断には使わない） ---
		bgLoad := fetchBackgroundLoad(client, *leafStats)

		// --- ここが分散バックプレッシャー伝播のコア ---
		if !*naive {
			ctrl.Update(avgLoad)
		}

		fmt.Printf("%d\t%.1f\t%.2f\t%.0f\t%d\t%d\n", tick, sendRate, avgLoad, bgLoad, okCount, failCount)

		// --- この tick の計測結果を記録（CSV出力用） ---
		records = append(records, metrics.Record{
			TimeS:   float64(tick),
			Rate:    sendRate,
			AvgLoad: avgLoad,
			BgLoad:  bgLoad,
			OK:      float64(okCount),
			Fail:    float64(failCount),
		})

		time.Sleep(1 * time.Second)
	}

	// --- 設定内容をファイル名に組み込んでCSVに書き出す ---
	filename := buildFilename(*naive, *fixedRate)

	f, err := os.Create(filename)
	if err != nil {
		fmt.Println("failed to create csv:", err)
		return
	}
	defer f.Close()

	if err := metrics.WriteCSV(f, records); err != nil {
		fmt.Println("failed to write csv:", err)
		return
	}

	fmt.Println("csv saved to:", filename)
	fmt.Println("visualize with: go run ./cmd/visualize -in=" + filename)
}

func modeLabel(naive bool) string {
	if naive {
		return "naive (fixed rate, no backpressure)"
	}
	return "AIMD (backpressure-aware)"
}

// fetchBackgroundLoad は Leaf の /stats エンドポイントを叩いて、
// 現在の背景負荷の同時実行数を取得する。取得に失敗したら 0 を返す
// （可視化用の補助情報であり、AIMDの判断には影響させないため）。
func fetchBackgroundLoad(client *http.Client, url string) float64 {
	resp, err := client.Get(url)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0
	}

	v, err := strconv.ParseFloat(string(body), 64)
	if err != nil {
		return 0
	}
	return v
}

// buildFilename は、実行時の設定（AIMDか固定か、固定ならそのレート）を
// 組み込んだCSVファイル名を組み立てる。
// 例: request_rate_aimd.csv / request_rate_naive_rate20.csv
func buildFilename(naive bool, fixedRate float64) string {
	mode := "aimd"
	if naive {
		mode = fmt.Sprintf("naive_rate%.0f", fixedRate)
	}
	return fmt.Sprintf("request_rate_%s.csv", mode)
}
