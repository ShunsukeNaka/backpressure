package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"
)

// capacity は大きめにしてある: このサービス自身はほぼ過負荷にならないようにするため
// （伝播が効いているかを観察しやすくするための意図的な設定）
const capacity = 50

const downstreamURL = "http://localhost:8081/"

var current int32

var downstreamClient = &http.Client{Timeout: 2 * time.Second}

func handler(w http.ResponseWriter, r *http.Request) {
	atomic.AddInt32(&current, 1)
	defer atomic.AddInt32(&current, -1)

	// 1. 自分自身の負荷を計算する（Leafサービスと同じロジック）
	ownLoad := float64(atomic.LoadInt32(&current)) / float64(capacity)
	if ownLoad > 1.0 {
		ownLoad = 1.0
	}

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

		if v, parseErr := strconv.ParseFloat(resp.Header.Get("X-Load"), 64); parseErr == nil {
			downstreamLoad = v
		}
		if resp.StatusCode != http.StatusOK {
			downstreamOK = false
		}
	}

	// --- ここがバックプレッシャー伝播のコア ---
	// 自分の負荷と下流の負荷のうち、大きい方を「実質的な負荷」として上流に伝える。
	// こうすることで、下流のどこか1段でも過負荷になれば、その情報が
	// このサービスを経由してさらに上流（Client側）まで伝わっていく。
	mergedLoad := ownLoad
	if downstreamLoad > mergedLoad {
		mergedLoad = downstreamLoad
	}

	w.Header().Set("X-Load", fmt.Sprintf("%.2f", mergedLoad))

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
