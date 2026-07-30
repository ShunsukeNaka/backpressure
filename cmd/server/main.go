package main

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"sync/atomic"
	"time"

	"backpressure-demo/pkg/backpressure"
)

// capacity は同時に処理できるリクエスト数の上限（これを超えると過負荷とみなす）
const capacity = 5

//処理中の件数を保持する変数（実リクエスト + 背景負荷の合計）。
// 負荷の計算そのものは pkg/backpressure の LoadTracker に委譲する。
var tracker = backpressure.NewLoadTracker(capacity)

// bgCurrent は、backgroundLoad が生成している「背景負荷」だけの
// 同時実行数を保持する変数。current とは別に持つことで、
// 「今の負荷のうちどれだけが背景負荷由来か」を外部から観測できるようにする。
var bgCurrent int32

func handler(w http.ResponseWriter, r *http.Request) {
	done := tracker.Begin() // 複数のリクエストが同時に操作を行ってもこの操作は絶対に他の処理と衝突しないことを保証してカウントを増やす（内部でatomicを使用）
	defer done()

	// 負荷率の計算（LoadTrackerに委譲）
	load := tracker.Load()

	// 負荷が高いほど処理が遅くなる（実際のサービスに近い挙動をシミュレート）
	baseMs := 20.0
	slowdown := 1.0 + load*4.0
	time.Sleep(time.Duration(baseMs*slowdown) * time.Millisecond)

	// これが本デモの核: 現在の負荷を1つの数値として返すだけ
	tracker.FormatHeader(w)

	if load >= 1.0 {
		w.WriteHeader(http.StatusServiceUnavailable) // 503
		w.Write([]byte("overloaded"))
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

// statsHandler は、現在の背景負荷の同時実行数を返すだけのエンドポイント。
// 可視化用にClient側から定期的に叩かれることを想定している。
func statsHandler(w http.ResponseWriter, r *http.Request) {
	bg := atomic.LoadInt32(&bgCurrent)
	fmt.Fprintf(w, "%d", bg)
}

// backgroundLoad は、テストクライアントとは無関係に発生する「他のサービスからの
// トラフィック」を模擬する。到着間隔・処理時間の両方にランダム性を持たせることで、
// 現実のように予測不能な負荷変動を作り出す。
func backgroundLoad(rate float64) {
	for {
		// ポアソン過程に近い到着間隔（指数分布）
		interval := time.Duration(rand.ExpFloat64() / rate * float64(time.Second))
		time.Sleep(interval)

		go func() {
			done := tracker.Begin()
			atomic.AddInt32(&bgCurrent, 1)
			defer done()
			defer atomic.AddInt32(&bgCurrent, -1)
			// 処理時間にもばらつきを持たせる（軽い処理〜重い処理が混在）
			time.Sleep(time.Duration(20+rand.Intn(300)) * time.Millisecond)
		}()
	}
}

func main() {
	go backgroundLoad(2.0) // 2 requests per second
	http.HandleFunc("/", handler)
	http.HandleFunc("/stats", statsHandler)
	log.Println("server listening on :8081 (capacity =", capacity, ")")
	log.Fatal(http.ListenAndServe(":8081", nil))
}
