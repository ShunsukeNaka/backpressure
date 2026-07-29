# backpressure-demo 用 Makefile
#
# 個別にサービスを起動するターゲットと、3プロセス(Leaf/Middle/Client)を
# まとめて動かして可視化まで行う `demo` ターゲットを用意している。

.PHONY: build run-server run-middle run-client run-client-naive \
        visualize demo demo-naive tidy clean help

# デフォルトターゲット: ヘルプを表示
help:
	@echo "使えるターゲット:"
	@echo "  make build              全コマンドをビルドする"
	@echo "  make run-server         Leafサービスを起動 (:8081)"
	@echo "  make run-middle         Middleサービスを起動 (:8082)"
	@echo "  make run-client         Clientを起動 (AIMDモード)"
	@echo "  make run-client-naive   Clientを起動 (naiveモード, FIXED_RATE=15)"
	@echo "  make visualize IN=xxx.csv   指定したCSVからグラフを生成"
	@echo "  make demo               Leaf/Middleをバックグラウンド起動し、"
	@echo "                          AIMDモードで計測してグラフまで自動生成"
	@echo "  make demo-naive         同上 (naiveモード, FIXED_RATE=15)"
	@echo "  make tidy               go mod tidy を実行"
	@echo "  make clean              生成された *.csv / *.png / バイナリを削除"

# 全コマンドをビルドしておく（動作確認用）
build:
	go build ./...

# --- 個別起動用のターゲット（複数ターミナルで手動実行する場合） ---

run-server:
	go run ./cmd/server

run-middle:
	go run ./cmd/middle

run-client:
	go run ./cmd/client

# FIXED_RATE は `make run-client-naive FIXED_RATE=20` のように上書きできる
FIXED_RATE ?= 15
run-client-naive:
	go run ./cmd/client -naive -fixed-rate=$(FIXED_RATE)

# IN は `make visualize IN=request_rate_aimd.csv` のように指定する
visualize:
	@if [ -z "$(IN)" ]; then \
		echo "IN=xxx.csv を指定してください（例: make visualize IN=request_rate_aimd.csv）"; \
		exit 1; \
	fi
	go run ./cmd/visualize -in=$(IN)

# --- demo: Leaf/Middleをバックグラウンドで起動し、Client実行後に自動で可視化まで行う ---
#
# 途中で失敗しても、または Ctrl+C で中断しても、バックグラウンドで起動した
# Leaf/Middleのプロセスは trap で確実に後始末する。

demo:
	@$(MAKE) _run_demo CLIENT_TARGET=run-client

demo-naive:
	@$(MAKE) _run_demo CLIENT_TARGET=run-client-naive

# _run_demo は内部専用ターゲット（直接は呼ばない）
_run_demo:
	@echo "Leaf/Middleをバックグラウンドで起動します..."
	@go run ./cmd/server & \
	SERVER_PID=$$!; \
	go run ./cmd/middle & \
	MIDDLE_PID=$$!; \
	trap "echo 'Leaf/Middleを停止します...'; kill $$SERVER_PID $$MIDDLE_PID 2>/dev/null" EXIT; \
	sleep 1; \
	echo "Clientを実行します ($(CLIENT_TARGET))..."; \
	$(MAKE) $(CLIENT_TARGET) FIXED_RATE=$(FIXED_RATE); \
	CSV=$$(ls -t request_rate_*.csv 2>/dev/null | head -n1); \
	if [ -n "$$CSV" ]; then \
		echo "グラフを生成します ($$CSV)..."; \
		$(MAKE) visualize IN=$$CSV; \
	else \
		echo "CSVが見つかりませんでした。"; \
	fi

# go.mod/go.sum を最新化する（gonum/plot などの依存追加後に実行）
tidy:
	go mod tidy

# 生成物の掃除
clean:
	rm -f request_rate_*.csv request_rate_*.png
	rm -rf bin/