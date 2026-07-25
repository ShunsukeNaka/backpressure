package main

import (
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
	"time"
)

// capacity は同時に処理できるリクエスト数の上限（これを超えると過負荷とみなす）
const capacity = 20
//処理中の件数を保持する変数
var current int32

func handler(w http.ResponseWriter, r *http.Request) {
	atomic.AddInt32(&current, 1) // 複数のリクエストが同時に操作を行ってもこの操作は絶対に他の処理と衝突しないことを保証してカウントを増やす
	defer atomic.AddInt32(&current, -1)

	load := float64(atomic.LoadInt32(&current)) / float64(capacity)
	if load > 1.0 {
		load = 1.0
	}

	// 負荷が高いほど処理が遅くなる（実際のサービスに近い挙動をシミュレート）
	baseMs := 20.0
	slowdown := 1.0 + load*4.0
	time.Sleep(time.Duration(baseMs*slowdown) * time.Millisecond)

	// これが本デモの核: 現在の負荷を1つの数値として返すだけ
	w.Header().Set("X-Load", fmt.Sprintf("%.2f", load))

	if load >= 1.0 {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("overloaded"))
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func main() {
	http.HandleFunc("/", handler)
	log.Println("server listening on :8081 (capacity =", capacity, ")")
	log.Fatal(http.ListenAndServe(":8081", nil))
}
