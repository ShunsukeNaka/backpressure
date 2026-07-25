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

## 次にやれること

- 3段目（Middle2）を追加して、Client → Middle1 → Middle2 → Leaf の3ホップにする
- 伝播にTTL（あと何ホップ伝播できるか）を持たせ、無限伝播を防ぐ
- 伝播頻度自体をスロットリングして、伝播コストが新たな負荷にならないようにする
- `X-Load` を単一指標ではなく複数指標にしてみる
- 自己申告(`ownLoad`)と実測(レイテンシ)の突合による信頼度チェックを入れる
