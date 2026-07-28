// Package metrics は、デモアプリケーション（cmd/client, cmd/visualize）の間で
// 計測結果をCSVとしてやり取りするための、共有スキーマを定義する。
//
// internal に置いているのは、これが backpressure-demo モジュールの外から
// 使われることを想定していない、デモ専用の付属コードだからである。
// 本物の配布用ライブラリは pkg/backpressure の方を参照。
package metrics

import (
	"encoding/csv"
	"io"
	"strconv"
)

// Record は、1tick分の計測結果を表す。
type Record struct {
	TimeS   float64
	Rate    float64
	AvgLoad float64
	BgLoad  float64
	OK      float64
	Fail    float64
}

var header = []string{"time_s", "rate", "avg_load", "bg_load", "ok", "fail"}

// WriteCSV は records を w に CSV 形式で書き出す（1行目はヘッダー）。
func WriteCSV(w io.Writer, records []Record) error {
	cw := csv.NewWriter(w)

	if err := cw.Write(header); err != nil {
		return err
	}

	for _, r := range records {
		row := []string{
			formatFloat(r.TimeS),
			formatFloat(r.Rate),
			formatFloat(r.AvgLoad),
			formatFloat(r.BgLoad),
			formatFloat(r.OK),
			formatFloat(r.Fail),
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}

	cw.Flush()
	return cw.Error()
}

// ReadCSV は r から CSV（WriteCSV が書いた形式）を読み込み、Record のスライスにする。
// ヘッダー行はスキップする。
func ReadCSV(r io.Reader) ([]Record, error) {
	cr := csv.NewReader(r)
	rows, err := cr.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) <= 1 {
		return nil, nil
	}

	records := make([]Record, 0, len(rows)-1)
	for _, row := range rows[1:] { // 1行目(ヘッダー)は読み飛ばす
		if len(row) < 6 {
			continue
		}
		records = append(records, Record{
			TimeS:   parseFloat(row[0]),
			Rate:    parseFloat(row[1]),
			AvgLoad: parseFloat(row[2]),
			BgLoad:  parseFloat(row[3]),
			OK:      parseFloat(row[4]),
			Fail:    parseFloat(row[5]),
		})
	}
	return records, nil
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func parseFloat(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}
