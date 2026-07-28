# 分散バックプレッシャー伝播 — 最小プロトタイプ（2ホップ版）

サービス間通信で、レスポンスに載せた単一の `X-Load` ヘッダーだけを頼りに、
呼び出し元が AIMD（Additive Increase / Multiplicative Decrease）で
送信レートを自動調整する、という発想を確認するためのコードです。

このバージョンでは **多段伝播** を試せるように、Client → Middle → Leaf の
3段構成にしています。Leaf（末端）が過負荷になったとき、その情報が
Middle を経由して Client にまで伝わり、Client と Middle の間は
まったく混んでいないのにレートが絞られる、という様子を観察できます。

## 動作確認

このサンドボックス環境には Go がインストールされておらず、ネットワークも
無効なため、コンパイル・実行の確認はできていません。標準ライブラリのみを
使っているので動くはずですが、手元の環境で一度動作確認してください。

## 構成

```
[Client] ──→ [Middle service :8082] ──→ [Leaf service :8081]
```

```
cmd/client/main.go   X-Load を見ながら AIMD でリクエストレートを調整するクライアント
cmd/middle/main.go   自分の負荷と下流(Leaf)の負荷を max() でマージして上流に伝える中間サービス
cmd/server/main.go   同時実行数から負荷率を計算し、X-Load ヘッダーで返すだけの末端(Leaf)サービス
```

意図的に容量差をつけています。

| サービス | capacity | 役割 |
|---|---|---|
| Leaf（`cmd/server`） | 5 | すぐ過負荷になる（ボトルネック役） |
| Middle（`cmd/middle`） | 50 | ほぼ過負荷にならない（伝播の中継役） |

## 伝播の仕組み

Middleサービスのhandlerは、リクエストを受けるたびに:

1. 自分自身の負荷（`ownLoad`）を計算する
2. 下流のLeafを呼び出し、Leafから返ってきた `X-Load`（`downstreamLoad`）を受け取る
3. `mergedLoad = max(ownLoad, downstreamLoad)` を計算する
4. `mergedLoad` を自分のレスポンスの `X-Load` として返す

これにより、Middle自身が全く混んでいなくても、Leafが混んでいれば
その情報がそのまま上流（Client）まで伝わります。

## 実行方法

ターミナルを3つ開いて、この順番で起動します。

```bash
# ターミナル1: Leaf（末端）起動
cd backpressure-demo
go run ./cmd/server

# ターミナル2: Middle（中間）起動
cd backpressure-demo
go run ./cmd/middle

# ターミナル3: Client起動（デフォルトでMiddle経由）
cd backpressure-demo
go run ./cmd/client
```

出力例（イメージ）:

```
time_s  rate    avg_load        ok      fail
0       1.0     0.10    1       0
1       2.0     0.15    2       0
...
5       8.0     0.85    6       2
6       4.0     0.60    4       0
```

Leaf側のcapacityが5と小さいので、比較的早い段階で `avg_load` が
上がり、Client側のレートが絞られるはずです。Middle自身のcapacityは
50なので、Middleの `current`（同時実行数）だけを見ればまだ余裕があるのに、
Clientのレートが落ちるのが確認できれば、伝播が機能している証拠です。

## 比較: 伝播なし（Leafに直接アクセス） vs 伝播あり（Middle経由）

```bash
# Middleを経由せず、Leafに直接アクセスした場合
go run ./cmd/client -target=http://localhost:8081/

# Middle経由（伝播あり、デフォルト）
go run ./cmd/client -target=http://localhost:8082/
```

Middleを挟んでも挟まなくても、最終的にLeafの過負荷はほぼ同じように
Clientに伝わるはずです。これは「中間ノードを経由しても、負荷情報が
正しく透過して伝わっている」ことの確認になります。

## 比較: バックプレッシャーなし（naive モード）

```bash
go run ./cmd/client -naive -fixed-rate=15
```

`fail`（503やタイムアウト）の数が AIMD ありのケースより明らかに増えるはずです。

## コアライブラリ（pkg/backpressure）

プロトコルのロジック（負荷計算、X-Loadヘッダーの読み書き、マージ、AIMD調整）は
`pkg/backpressure` という1つのGoパッケージに切り出してあります。これは他のプロジェクトが
importして使うことを想定した、本当の意味での配布用ライブラリです。

```go
import "backpressure-demo/pkg/backpressure"

// サーバー側: 負荷を計算してヘッダーに乗せる
tracker := backpressure.NewLoadTracker(capacity)
done := tracker.Begin()
defer done()
tracker.FormatHeader(w) // X-Load ヘッダーをセット

// 中間サービス側: 自分の負荷と下流の負荷をマージする
merged := backpressure.Merge(ownLoad, downstreamLoad)

// クライアント側: AIMDでレートを自動調整する
ctrl := backpressure.NewAIMDController(1.0)
newRate := ctrl.Update(avgLoad)
```

## デモアプリケーションと計測・可視化の分離

`cmd/server`, `cmd/middle`, `cmd/client`, `cmd/visualize` は、いずれも
`pkg/backpressure` の使い方を示すための**デモアプリケーション**であり、
それ自体は配布用ライブラリではありません。

このうち「計測（cmd/client）」と「可視化（cmd/visualize）」は、責務を分けて
別コマンドにしてあります。

```bash
go run ./cmd/client                        # 計測してCSVに出力するだけ
go run ./cmd/visualize -in=request_rate_aimd.csv  # CSVを読んでPNGにするだけ
```

計測と可視化を分けている理由:

- 生データ（CSV）が手元に残るので、後からグラフのスタイルを変えたり、
  複数回の実行結果を比較したりが自由にできる
- `gonum/plot`（可視化ライブラリ）への依存を`cmd/visualize`だけに閉じ込められる
- 将来、可視化をGo以外（Python/matplotlibなど）に置き換えたくなっても、
  CSVを読ませるだけで済む

CSVの読み書きスキーマ（列の定義）は、`cmd/client`と`cmd/visualize`の両方から
参照される `internal/metrics` にまとめています。`internal/` に置いているのは、
これがこのモジュールの外から使われることを想定していない、デモ専用の
共有コードだからです（`pkg/backpressure`とは意図的に区別しています）。

## ディレクトリ構成

```
pkg/backpressure/     プロトコルのコアロジック（配布用ライブラリ）
internal/metrics/     CSVスキーマの共有コード（デモ専用、モジュール外からは使えない）
cmd/server/            デモ: Leafサービス
cmd/middle/            デモ: Middleサービス
cmd/client/            デモ: 計測してCSVに出力するクライアント
cmd/visualize/         デモ: CSVを読んでPNGにする可視化ツール
```