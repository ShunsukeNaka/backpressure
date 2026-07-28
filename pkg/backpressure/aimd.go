package backpressure

// AIMDController は、TCPの輻輳制御と同じ考え方（Additive Increase /
// Multiplicative Decrease）で、送信レートを自動調整するための道具です。
//
//	ctrl := backpressure.NewAIMDController(1.0)
//	for each tick {
//	    avgLoad := ... // このtickで観測した平均負荷
//	    rate := ctrl.Update(avgLoad)
//	    // rate 件のリクエストを送る
//	}
type AIMDController struct {
	// Rate は現在の送信レート（1秒あたりの件数など、単位は呼び出し側が決める）。
	Rate float64

	// MinRate は Rate がこれより小さくならないようにする下限。
	MinRate float64

	// DecreaseThreshold を超える負荷を観測したら、乗算的に減少させる。
	DecreaseThreshold float64
	// DecreaseFactor は減少時に Rate に掛ける係数（例: 0.5で半減）。
	DecreaseFactor float64

	// IncreaseThreshold を下回る負荷を観測したら、加算的に増加させる。
	IncreaseThreshold float64
	// IncreaseStep は増加時に Rate に足す量。
	IncreaseStep float64
}

// NewAIMDController は、一般的なデフォルト値
// （閾値 0.3〜0.8、半減、+1ずつ増加、下限1.0）で初期化した
// コントローラーを作る。フィールドを直接上書きすればチューニング可能。
func NewAIMDController(initialRate float64) *AIMDController {
	return &AIMDController{
		Rate:              initialRate,
		MinRate:           1.0,
		DecreaseThreshold: 0.8,
		DecreaseFactor:    0.5,
		IncreaseThreshold: 0.3,
		IncreaseStep:      1.0,
	}
}

// Update は観測した平均負荷 avgLoad を受け取り、AIMDのルールに従って
// Rate を更新し、更新後の値を返す。
//
//   - avgLoad > DecreaseThreshold なら、Rate を DecreaseFactor 倍する（急減）
//   - avgLoad < IncreaseThreshold なら、Rate に IncreaseStep を足す（緩増）
//   - それ以外（安全域）なら Rate は変化しない
func (c *AIMDController) Update(avgLoad float64) float64 {
	switch {
	case avgLoad > c.DecreaseThreshold:
		c.Rate = c.Rate * c.DecreaseFactor
		if c.Rate < c.MinRate {
			c.Rate = c.MinRate
		}
	case avgLoad < c.IncreaseThreshold:
		c.Rate = c.Rate + c.IncreaseStep
	}
	return c.Rate
}
