package main

import (
	"io"
	"log"
	"net/http"
	"time"

	"backpressure-demo/pkg/backpressure"
)

// capacity は大きめにしてある: このサービス自身はほぼ過負荷にならないようにするため
// （伝播が効いているかを観察しやすくするための意図的な設定）
const capacity = 50

const downstreamURL = "http://localhost:8081/"

var tracker = backpressure.NewLoadTracker(capacity)

var downstreamClient = &http.Client{Timeout: 2 * time.Second}

func handler(w http.ResponseWriter, r *http.Request) {
	done := tracker.Begin()
	defer done()

	// 1. 自分自身の負荷を計算する（コアライブラリに委譲）
	ownLoad := tracker.Load()

	// 2. 下流（Leafサービス）を呼び出し、その負荷を受け取る
	downstreamLoad := 0.0
	downstreamOK := true

	resp, err := downstreamClient.Get(downstreamURL)
	if err != nil {
		// 下流に届かない・タイムアウト = 実質的に最大負荷とみなす
		downstreamLoad = 1.0
		downstreamOK = false
	} else {
		defer resp.Body.Close()
		io.Copy(io.Discard, resp.Body)

		if v, parseErr := backpressure.ParseLoad(resp); parseErr == nil {
			downstreamLoad = v
		}
		if resp.StatusCode != http.StatusOK {
			downstreamOK = false
		}
	}

	// --- ここがバックプレッシャー伝播のコア（コアライブラリのMergeに委譲） ---
	// 自分の負荷と下流の負荷のうち、大きい方を「実質的な負荷」として上流に伝える。
	// こうすることで、下流のどこか1段でも過負荷になれば、その情報が
	// このサービスを経由してさらに上流（Client側）まで伝わっていく。
	mergedLoad := backpressure.Merge(ownLoad, downstreamLoad)

	w.Header().Set(backpressure.HeaderName, backpressure.FormatLoad(mergedLoad))

	if !downstreamOK || mergedLoad >= 1.0 {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("downstream overloaded"))
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func main() {
	http.HandleFunc("/", handler)
	log.Println("middle service listening on :8082 (capacity =", capacity, "), forwarding to", downstreamURL)
	log.Fatal(http.ListenAndServe(":8082", nil))
}
