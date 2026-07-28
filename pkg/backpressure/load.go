// Package backpressure は、分散バックプレッシャー伝播プロトコルの
// コアロジック（負荷の計算・伝達・AIMDによる調整）をまとめたライブラリです。
//
// サーバー側は LoadTracker で自分の負荷を計算して X-Load ヘッダーに乗せ、
// クライアント側は AIMDController でそのヘッダーを見ながら送信レートを
// 自動調整します。中間サービス（プロキシ的な役割）は、両方を組み合わせて
// 自分の負荷と下流から受け取った負荷を Merge することで、多段の伝播を実現します。
package backpressure

import (
	"fmt"
	"net/http"
	"strconv"
	"sync/atomic"
)

// HeaderName は、負荷情報を伝えるために使う HTTP ヘッダー名。
const HeaderName = "X-Load"

// LoadTracker は「今何件同時に処理中か」を数え、capacity に対する
// 負荷率（0.0〜1.0）を計算するための道具です。並行アクセスに対して安全です。
type LoadTracker struct {
	capacity int32
	current  int32
}

// NewLoadTracker は、同時処理の上限が capacity のトラッカーを作る。
func NewLoadTracker(capacity int32) *LoadTracker {
	return &LoadTracker{capacity: capacity}
}

// increment は current を delta だけ変化させ、変化後の値を返す。
// atomic.AddInt32 を使うことで、複数のリクエストが同時に呼んでも安全。
func (t *LoadTracker) increment(delta int32) int32 {
	return atomic.AddInt32(&t.current, delta)
}

// currentValue は現在の同時処理件数を安全に読み出す。
func (t *LoadTracker) currentValue() int32 {
	return atomic.LoadInt32(&t.current)
}

// Begin は「1件処理を開始した」ことを記録し、処理が終わったら呼ぶべき
// 終了関数を返す。呼び出し側は defer で終了関数を予約するだけでよい。
//
//	done := tracker.Begin()
//	defer done()
func (t *LoadTracker) Begin() (done func()) {
	t.increment(1)
	return func() { t.increment(-1) }
}

// Load は現在の負荷率（0.0〜1.0）を返す。
func (t *LoadTracker) Load() float64 {
	load := float64(t.currentValue()) / float64(t.capacity)
	if load > 1.0 {
		load = 1.0
	}
	if load < 0 {
		load = 0
	}
	return load
}

// FormatHeader は、現在の負荷を X-Load ヘッダー用の文字列にして
// ResponseWriter にセットする。
func (t *LoadTracker) FormatHeader(w http.ResponseWriter) {
	w.Header().Set(HeaderName, FormatLoad(t.Load()))
}

// FormatLoad は負荷率(0.0〜1.0)を X-Load ヘッダー用の文字列に変換する。
func FormatLoad(load float64) string {
	return fmt.Sprintf("%.2f", load)
}

// ParseLoad は、レスポンスから受け取った X-Load ヘッダーの文字列を
// 数値の負荷率にパースする。ヘッダーが空・不正な場合はエラーを返す。
func ParseLoad(resp *http.Response) (float64, error) {
	raw := resp.Header.Get(HeaderName)
	if raw == "" {
		return 0, fmt.Errorf("backpressure: %s header is missing", HeaderName)
	}
	return strconv.ParseFloat(raw, 64)
}

// Merge は複数の負荷値のうち最大値を返す。
//
// 中間サービスが「自分の負荷」と「下流から受け取った負荷」をマージして
// 上流に伝えるときに使う。最大値を取ることで、経路上のどこか1段でも
// 過負荷になれば、その情報がそのまま上流まで伝播する（経路圧縮）。
func Merge(loads ...float64) float64 {
	max := 0.0
	for _, l := range loads {
		if l > max {
			max = l
		}
	}
	return max
}
