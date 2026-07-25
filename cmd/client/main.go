package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"
)

func main() {
	naive := flag.Bool("naive", false, "AIMDを使わず固定レートで送り続ける（比較用）")
	fixedRate := flag.Float64("fixed-rate", 15.0, "-naive 使用時の固定リクエストレート")
	ticks := flag.Int("ticks", 40, "実行する秒数")
	target := flag.String("target", "http://localhost:8082/", "呼び出し先URL（デフォルトはMiddle経由。Leafに直接投げるなら http://localhost:8081/ を指定）")
	flag.Parse()

	rate := 1.0
	const minRate = 1.0

	client := &http.Client{Timeout: 2 * time.Second}

	fmt.Println("mode:", modeLabel(*naive))
	fmt.Println("time_s\trate\tavg_load\tok\tfail")

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

		time.Sleep(1 * time.Second)
	}
}

func modeLabel(naive bool) string {
	if naive {
		return "naive (fixed rate, no backpressure)"
	}
	return "AIMD (backpressure-aware)"
}
