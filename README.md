# Go API テンプレートの使い方

Go と PostgreSQL で API サーバーを作るためのテンプレートです。HTTP ルーティングに chi、DB 接続に pgxpool、ログに標準ライブラリの slog を使います。ORM は使わず、サンプルとしてタスクの作成・一覧・取得・削除 API を実装しています。

ローカルでは `.env` を使い、本番では実行環境から環境変数を渡します。Docker イメージのビルドに `.env` や DB 接続情報は不要です。

## 1 ローカル開発の準備

次のツールを用意してください。コマンド例は macOS・Linux・WSL などのシェル向けです。

- Go：`go.mod` の指定は 1.23.0 以上です。
- Docker と Docker Compose：`docker compose` が実行できること。
- make
- curl：API の動作確認に使います。

作業ディレクトリを、この README がある場所に移動します。ワークスペースのルートからなら次のコマンドです。

```sh
cd api-template/go-api-template
```

初回は依存関係を取得します。ネットワーク接続が必要です。

```sh
go mod download
```

`go.mod` と `go.sum` は同梱しています。依存関係を変更した場合は `go mod tidy` を実行し、両方をコミットしてください。

## 2 ローカル用の環境変数を用意する

`.env` がまだない場合に、サンプルをコピーします。既存の `.env` はそのまま使えます。

```sh
if [ ! -f .env ]; then cp .env.example .env; fi
```

標準設定は次のとおりです。

| 変数 | 初期値 | 用途 |
| --- | --- | --- |
| `PORT` | `8080` | ホスト側で API にアクセスするポート |
| `POSTGRES_PORT` | `5432` | ホスト側で PostgreSQL に接続するポート |
| `POSTGRES_USER` | `app` | ローカル DB のユーザー名 |
| `POSTGRES_PASSWORD` | `app_dev_password` | ローカル DB のパスワード |
| `POSTGRES_DB` | `appdb` | ローカル DB 名 |
| `DATABASE_URL` | 他の変数から組み立てる接続 URL | ホストで起動する API の DB 接続先 |
| `DB_MAX_CONNS` | `5` | API プロセスごとの最大 DB 接続数 |

API はホスト上で起動し、サンプルの `DATABASE_URL` で `localhost` の DB に接続します。マイグレーション用コンテナは `db:5432` に接続します。

別の API や DB とポートが競合する場合は、`.env` の `PORT` や `POSTGRES_PORT` を変更してください。DB のユーザー名・パスワード・DB 名は、DB ボリュームの初期作成時に適用されます。既存ボリュームに対して `.env` だけを変更しても、DB 内の認証情報は変わりません。

`.env` はシェルから読み込める `KEY=value` 形式で記述してください。Git と Docker のビルド対象から除外されており、ローカル開発専用です。

## 3 DB と API を起動する

Docker を起動した状態で、順に実行します。

```sh
# PostgreSQL をバックグラウンドで起動
make db-up

# テーブルを作成
make migrate-up

# .env を読み込み、ホスト上で API を起動
make dev
```

`make dev` を実行したターミナルは開いたままにします。標準設定では `http://localhost:8080` にアクセスできます。ソースを変更した場合は `Ctrl+C` で終了し、再度 `make dev` を実行してください。

直接 `go run ./cmd/api` を実行する場合、`.env` は自動では読み込まれません。通常は `make dev` を使ってください。API は起動時に DB 接続を確認するため、DB に接続できないと起動しません。

## 4 API の動作を確認する

別のターミナルで実行します。ポートを変更した場合は、URL の `8080` も変更してください。

```sh
# API プロセスの応答を確認
curl -i http://localhost:8080/health

# DB への接続を確認
curl -i http://localhost:8080/ready

# タスクを作成
curl -i -X POST http://localhost:8080/api/v1/tasks \
  -H 'Content-Type: application/json' \
  -d '{"title":"Go API を試す"}'

# タスク一覧を取得
curl -i http://localhost:8080/api/v1/tasks
```

正常時、ヘルスチェックと一覧は `200`、作成は `201` を返します。作成レスポンスの `id` を確認し、次の `TASK_ID` に設定します。

```sh
TASK_ID=1 # 実際に返された id に置き換える

# 1 件取得
curl -i "http://localhost:8080/api/v1/tasks/$TASK_ID"

# 削除
curl -i -X DELETE "http://localhost:8080/api/v1/tasks/$TASK_ID"
```

削除成功時は `204` で、レスポンス本文はありません。存在しない ID は `404` になります。更新 API はサンプルに含まれていません。

### タスク一覧の続きを取得する

このブランチでは、既存の一覧 API に `limit` と `after_id` を追加しています。返す JSON は従来どおりタスクの配列です。

```sh
# 最初の 2 件を取得（id の昇順）
curl -i 'http://localhost:8080/api/v1/tasks?limit=2'

# 直前の配列の最後の id に置き換えて、続きの 2 件を取得
AFTER_ID=2
curl -i "http://localhost:8080/api/v1/tasks?limit=2&after_id=$AFTER_ID"
```

| クエリ | 省略時 | 指定できる値 |
| --- | --- | --- |
| `limit` | `100` | `1`〜`100` の整数 |
| `after_id` | `0` | `0`〜`9223372036854775807` の整数 |

`after_id` より大きい ID を最大 `limit` 件返します。`after_id` のタスクが削除されても続きを取得でき、続きがなければ `200` と空配列 `[]` を返します。クエリを付けなければ、従来どおり先頭から最大 100 件です。

空の指定（`?limit=` や `?after_id`）、範囲外、整数以外、同じキーの複数指定、不正な URL エンコードは `400` です。総件数や次ページ用の専用フィールドは追加せず、最後の ID を呼び出し側で保持します。ページの取得中に追加・削除されたデータはその時点の状態で返すため、全ページを固定したスナップショットではありません。

## 5 テストと終了

```sh
make test
```

通常のテストは DB なしで実行できます。HTTP 経由の CRUD とページングを PostgreSQL と合わせて確認する場合は、テスト用 DB の接続先を指定します。

```sh
TEST_DATABASE_URL='postgres://app:app_dev_password@localhost:5432/appdb?sslmode=disable' go test ./internal/app -run '^TestTaskAPI$' -v
```

この統合テストは専用の一時スキーマにマイグレーションを適用し、終了時にそのスキーマだけを削除します。既存のテーブルやデータは変更しません。接続ユーザーにはスキーマ作成権限が必要です。`TEST_DATABASE_URL` が未指定の場合、このテストだけスキップします。

API は実行中のターミナルで `Ctrl+C` を押して終了します。DB などの Compose コンテナを停止・削除する場合は、次を実行します。DB データのボリュームは残ります。

```sh
docker compose --profile tools down
```

次回は `make db-up` と `make dev` で再開できます。マイグレーションを追加した場合は、API を起動する前に `make migrate-up` も実行してください。

## 6 Docker イメージを作る

プロジェクトのディレクトリで実行します。

```sh
make build
```

`go-api-template:local` というイメージが作られます。直接実行する場合は次のコマンドです。

```sh
docker build -f docker/Dockerfile.prod -t go-api-template:local .
```

この工程では `.env`、`DATABASE_URL`、稼働中の DB は不要です。依存パッケージとベースイメージの取得にはネットワーク接続が必要です。`.dockerignore` が環境変数ファイルを除外し、本番 Dockerfile はビルド用に Go のソースディレクトリだけをコピーします。

完成したイメージを起動するときには、コンテナから接続可能な `DATABASE_URL` が必要です。ホスト起動用の `.env` にある `localhost` は、コンテナ内ではそのコンテナ自身を指すため、そのまま流用しないでください。

## 7 本番環境へリリースする

GitHub Actions を追加するときは、次の順序にします。ワークフロー自体はこのテンプレートにまだ含まれていません。

1. テストを実行する。
2. `docker/Dockerfile.prod` を指定してイメージをビルドする。
3. Artifact Registry などにイメージを push する。
4. 本番 DB に対して、専用のジョブなどからマイグレーションを実行する。
5. 実行環境に環境変数とシークレット参照を設定し、イメージをデプロイする。

通常の設定値はデプロイ用 YAML やデプロイコマンドから、DB 接続情報は GCP Secret Manager から環境変数として渡します。GitHub Actions のジョブに環境変数を設定するだけでは、Cloud Run の実行環境には引き継がれません。[Cloud Run の環境変数設定](https://cloud.google.com/run/docs/configuring/services/environment-variables)でサービス側にも設定してください。

Cloud Run では、実行用サービスアカウントに対象シークレットの `roles/secretmanager.secretAccessor` 権限を付与します。Secret Manager の値を環境変数として使う場合は、バージョンを固定するとリリースごとの設定を管理できます。[Cloud Run のシークレット設定](https://cloud.google.com/run/docs/configuring/services/secrets)

GCP の認証、Artifact Registry のリポジトリ、本番 DB とその接続経路、シークレットを準備済みの場合のコマンド例です。値は自分の環境に置き換えてください。

```sh
PROJECT_ID="your-gcp-project"
REGION="asia-northeast1"
REPOSITORY="api-images"
SERVICE="go-api-example"
IMAGE_TAG="release-001" # リリースごとに変える。CI ではコミット SHA などを使用
SECRET_NAME="go-api-database-url"
SECRET_VERSION="1"
IMAGE="$REGION-docker.pkg.dev/$PROJECT_ID/$REPOSITORY/$SERVICE:$IMAGE_TAG"

gcloud auth configure-docker "$REGION-docker.pkg.dev"
docker build --platform linux/amd64 -f docker/Dockerfile.prod -t "$IMAGE" .
docker push "$IMAGE"

# 本番 DB のマイグレーション完了後に実行
gcloud run deploy "$SERVICE" \
  --image "$IMAGE" \
  --project "$PROJECT_ID" \
  --region "$REGION" \
  --update-secrets "DATABASE_URL=$SECRET_NAME:$SECRET_VERSION" \
  --update-env-vars DB_MAX_CONNS=5
```

Cloud Run 用の DB ネットワーク設定は別途必要です。Cloud SQL の Unix ソケット接続を使う構成では、インスタンス接続設定と実行用サービスアカウントの Cloud SQL Client 権限も用意してください。

アプリは実行環境の `PORT` を使い、全インターフェースで待ち受けます。DB の最大接続数はインスタンスごとに適用されるため、スケール後の合計接続数も考慮してください。サンプル API には認証がないので、公開範囲と認証を設定してから本番データを扱ってください。

`make deploy` も用意されています。既存の環境変数やシークレット設定を保持し、`DATABASE_URL` と `DB_MAX_CONNS` を更新します。呼び出し元の環境変数または make の引数で `GCP_PROJECT`、`GCP_REGION`、`AR_REPOSITORY`、`SERVICE_NAME`、`DATABASE_URL_SECRET` を渡します。ローカルの `.env` は読み込みません。この簡易コマンドはイメージとシークレットに `latest` を使うため、バージョンを固定したリリースには上記の手順を使ってください。

## 8 新しい API サーバーとして使う

テンプレートを別のプロジェクトとしてコピーしたら、モジュール名を変更します。

```sh
go mod edit -module example.com/my-api
```

続いてエディタの一括置換で、Go ファイル内の import パスの `example.com/go-api-template` を `example.com/my-api` に変更します。`go mod edit` だけでは import パスは変わりません。

```sh
go mod tidy
make test
```

サンプルの `tasks` を目的のリソースに置き換えます。

| 場所 | 役割 |
| --- | --- |
| `cmd/api/main.go` | 起動処理と終了処理 |
| `internal/app/app.go` | ルーティングと HTTP サーバー設定 |
| `internal/config` | 環境変数の読み込み |
| `internal/database` | DB 接続プール |
| `internal/common/json.go` | JSON の読み書きとエラーレスポンス |
| `internal/resources/tasks` | サンプルのモデル・ハンドラー・SQL |
| `migrations` | DB スキーマの変更 |

リソースを追加するときは `internal/resources/<リソース名>` に実装し、`internal/app/app.go` にルートを登録します。業務ロジックが増えた段階で `service.go` を追加してください。

新しい DB で使い始める前ならサンプルのマイグレーションを置き換えられます。運用開始後は適用済みのファイルを編集せず、新しい番号のマイグレーションを追加してください。API はマイグレーションを自動実行しません。

## このブランチの差分を読む

`example/task-pagination` は、`main` の既存 API にクエリを追加する開発例です。DB のスキーマや依存パッケージの変更はありません。差分は次の順に読むと、HTTP の入力から SQL まで追えます。

1. `internal/resources/tasks/handler.go`：クエリの省略値・入力検証を追加し、値をリポジトリに渡す。
2. `internal/resources/tasks/repository.go`：`List` の引数を増やし、SQL に取得位置と件数を渡す。
3. `internal/resources/tasks/handler_test.go`：DB を使わず、空指定・範囲外・重複などを拒否することを確認する。
4. `internal/app/app_integration_test.go`：実 DB で連続ページ、空の終端、削除済み ID からの再開を確認する。
5. `README.md`：呼び出し方と境界条件を更新する。

```sh
git diff main...example/task-pagination
```

## コマンド一覧

| コマンド | 内容 |
| --- | --- |
| `make db-up` | ローカル PostgreSQL の起動 |
| `make migrate-up` | 未適用のローカル DB マイグレーションを適用 |
| `make migrate-down` | ローカル DB のマイグレーションを一つ戻す。サンプルでは tasks テーブルとデータが削除される |
| `make dev` | `.env` を読み込んでホスト上で API を起動 |
| `make test` | Go のテストを実行 |
| `make build` | 本番用 Docker イメージを作成 |
| `make deploy` | イメージのビルド・push と Cloud Run へのデプロイ |

## 各ファイルの役割と変更する場面

### 起動からリクエスト処理までの流れ

起動時は `main.go` が設定を読み、DB 接続プールを作ってから HTTP サーバーを起動します。接続プールは、複数のリクエストで DB 接続を再利用するための仕組みです。

```text
cmd/api/main.go
  → config.Load()       環境変数を読み込む
  → database.Open()     DB 接続プールを作り、接続を確認する
  → app.Run()           HTTP サーバーを起動する
      → app.Router()    URL と処理を結び付ける
```

タスク作成リクエストでは、次の順序で処理します。

```text
POST /api/v1/tasks
  → app.go              共通処理を通し、tasks のルートへ振り分ける
  → tasks/handler.go    JSON を読み、入力値を検証する
  → tasks/repository.go SQL を実行する
  → PostgreSQL          データを保存し、結果を返す
  → tasks/handler.go    結果を受け取る
  → common/json.go      HTTP ステータスと JSON を返す
```

ハンドラーは HTTP の入出力、リポジトリは SQL と DB の入出力を担当します。たとえば URL の変更はハンドラー側、取得条件の変更はリポジトリ側で行います。

### アプリケーションの起動と共通処理

**[cmd/api/main.go](cmd/api/main.go)**

アプリケーションの入口です。JSON 形式のログ出力、設定の読み込み、DB 接続、HTTP サーバー起動を組み立てます。起動時の DB 接続確認には 10 秒の期限を設定し、`SIGINT` と `SIGTERM` を受け取ると終了処理に進みます。通常終了時は `defer db.Close()` で DB 接続を閉じます。エラー時の `os.Exit(1)` では defer は実行されません。

起動時に外部サービスのクライアントを作るなど、アプリ全体の初期化を追加するときに変更します。

**[internal/config/config.go](internal/config/config.go)**

`Config` 構造体と `Load()` を定義します。`PORT`、`DATABASE_URL`、`DB_MAX_CONNS` を環境変数から読み、DB 接続 URL の必須チェックと最大接続数の正数チェックを行います。ヘッダー読み取りの期限は 5 秒、終了待ち時間は 10 秒で、現状はコード内の固定値です。

設定項目を増やすときは、このファイルにフィールド・読み込み・検証を追加します。ここでは `.env` を開きません。ローカル用ファイルの読み込みは Makefile と Compose が担当し、Go のコードは渡された環境変数だけを参照します。

**[internal/config/config_test.go](internal/config/config_test.go)**

`DATABASE_URL` が空のとき、`Load()` がエラーになることを確認するテストです。設定値の検証を追加したら、その正常値・異常値のテストもここに追加します。現在はこの一つのケースだけを確認しています。

**[internal/database/database.go](internal/database/database.go)**

`Open()` が PostgreSQL の接続設定を解析し、最大接続数を設定して pgxpool の接続プールを作ります。`Ping()` による接続確認に失敗した場合は、作成したプールを閉じてエラーを返します。

接続プールの寿命や接続設定を調整するときに変更します。リソースごとの SQL はここに置かず、それぞれの `repository.go` に記述します。

**[internal/app/app.go](internal/app/app.go)**

`Router()` と `Run()` を定義します。`Router()` はリクエスト ID、panic の回復、30 秒の処理期限、HTTP ステータス付きのアクセスログを設定し、ヘルスチェックとタスク API のルートを登録します。`/health` はプロセスの応答、`/ready` は 2 秒の期限で DB 接続を確認します。30 秒の期限は context による通知で、処理を強制的に停止するものではありません。

`Run()` は HTTP サーバーを起動し、終了通知を受けたら `Shutdown()` で処理中のリクエストの完了を待ちます。待機期限を超えた場合などは `Close()` を呼びます。新しいリソースの登録、認証などの共通処理の追加、HTTP サーバー設定の変更を行う場所です。

**[internal/common/json.go](internal/common/json.go)**

各ハンドラーから使う JSON の共通処理です。

| 定義 | 役割 |
| --- | --- |
| `APIError` | `error` と `request_id` を持つエラーレスポンスの型 |
| `JSON()` | Content-Type、HTTP ステータス、JSON 本文を返す |
| `Error()` | リクエスト ID を付けてエラーを JSON で返す |
| `DecodeJSON()` | リクエスト本文を Go の構造体などに読み込む |

`DecodeJSON()` は読み込む本文を 1 MiB に制限し、複数の JSON 値が連続した本文を拒否します。`DisallowUnknownFields()` は、読み込み先の構造体にないフィールドを拒否する指定です。たとえば `CreateTask` のフィールドは `title` だけなので、`{"title":"test","unexpected":true}` はエラーになります。

必須項目や文字数などの業務上の入力チェックは、JSON の形式チェックとは別にハンドラーが行います。共通のレスポンス形式や JSON の受け付け方を変更するときは、このファイルを変更します。現在の `JSON()` はエンコード時のエラーを呼び出し元へ返さない実装です。

### サンプルのタスク API

**[internal/resources/tasks/model.go](internal/resources/tasks/model.go)**

タスクを表す `Task` と、作成リクエストを表す `CreateTask` を定義します。`Task` は ID・タイトル・完了状態・作成日時を持ち、`json` タグで JSON のフィールド名を指定しています。入力用の `CreateTask` はタイトルだけを持ち、ID や作成日時は DB 側で決まります。

API の入力・出力項目を追加するときに変更します。DB に保存する項目を増やす場合は、SQL とマイグレーションも合わせて変更します。

**[internal/resources/tasks/handler.go](internal/resources/tasks/handler.go)**

HTTP リクエストを受け取り、リポジトリを呼び出してレスポンスを返します。`Routes()` が一覧・作成・取得・削除のルートを定義し、`app.go` がその前に `/api/v1/tasks` を付けて登録します。`parseID()` は ID が正の整数かどうかを確認します。

作成時はタイトル前後の空白を除去し、空文字と長さ超過を拒否します。`utf8.RuneCountInString()` によって Unicode コードポイント単位で 1〜200 文字を受け付けます。JSON の解析失敗は `400`、対象がない場合は `404`、DB などの内部エラーは原因とリクエスト ID をログに記録し、クライアントには詳細を含めず `500` を返します。

エンドポイント、入力チェック、HTTP ステータスを変更する場所です。複雑な業務ロジックが増えたら、同じディレクトリに `service.go` を追加して切り出せます。

**[internal/resources/tasks/repository.go](internal/resources/tasks/repository.go)**

`List()`、`Create()`、`Get()`、`Delete()` で SQL を実行します。入力値は `$1` などのパラメーターとして渡し、結果を `Scan()` で `Task` に読み込みます。取得・削除の対象がない場合は `ErrNotFound` を返し、ハンドラーがそれを `404` に変換します。

一覧の `List(ctx, afterID, limit)` は、`WHERE id > $1 ORDER BY id ASC LIMIT $2` によって指定 ID より後のタスクを返します。`afterID` と `limit` はハンドラーが検証した値を渡します。検索条件、並び順、ページング、保存する項目を変更するときに編集します。

### DB スキーマ

**[migrations/000001_create_tasks.up.sql](migrations/000001_create_tasks.up.sql)**

最初のマイグレーションです。`tasks` テーブルを作り、自動採番の ID、タイトル、初期値が false の完了状態、作成日時を定義します。`make migrate-up` で適用します。

**[migrations/000001_create_tasks.down.sql](migrations/000001_create_tasks.down.sql)**

上記の変更を戻す SQL です。`tasks` テーブルを削除するため、保存されたデータも失われます。`make migrate-down` でこのマイグレーションを戻すときに実行されます。スキーマを拡張するときは、次の番号の up・down ファイルを追加します。

### 開発環境とビルド

**[go.mod](go.mod)**

Go のモジュール名、必要な Go のバージョン、依存ライブラリを宣言します。現在の主な依存は chi と pgx です。プロジェクト名や依存関係を変えるときに更新します。

`go mod tidy` で生成される `go.sum` は、依存モジュールの検証に使うチェックサムを記録します。こちらも Git に含めます。

**[Makefile](Makefile)**

起動・マイグレーション・テスト・ビルド・デプロイのコマンドを短い名前にまとめます。`make dev` だけがシェル経由で `.env` を読み込みます。`make build` と `make deploy` は `.env` を読み込みません。

よく使う開発コマンドを追加したり、ビルドやリリースの呼び出し方を変更したりするときに編集します。

**[compose.yaml](compose.yaml)**

ローカル用の PostgreSQL とマイグレーション用コンテナを定義します。DB データは `pgdata` ボリュームに保存します。マイグレーションは `tools` プロファイルで起動し、DB のヘルスチェック成功を待ちます。API は `make dev` でホスト上に起動します。

ポート、ローカル DB、開発用コンテナの設定を変更するときに編集します。ここに書いた環境変数はローカルのコンテナ向けで、本番イメージや Cloud Run に自動では引き継がれません。

**[docker/Dockerfile.prod](docker/Dockerfile.prod)**

本番用イメージを二段階で作ります。最初の段階で Linux 用の Go 実行ファイルをビルドし、次の段階でその実行ファイルだけを distroless イメージにコピーします。実行ユーザーは root ではありません。

本番イメージには Go のビルドツールやマイグレーション SQL は入りません。Go や実行用ベースイメージの更新、ビルドオプションの変更をするときに編集します。環境変数はコンテナ起動時に渡します。

### 環境変数と除外設定

**[.env.example](.env.example)**

Git に含めるローカル設定のひな形です。必要な環境変数が増えたら、開発用の値や説明を追加します。本番のシークレットは記載しません。

**`.env`**

開発者が手元で使う実際の設定ファイルです。`.env.example` からコピーし、ポートなどを各自の環境に合わせます。`make dev` と Docker Compose が読み込み、Git と Docker のビルド対象からは除外されます。

**[.gitignore](.gitignore)**

Git に追加しないファイルを指定します。`.env` とその派生ファイル、`tmp/`、`.DS_Store` を除外し、`.env.example` は例外として共有します。ローカルの生成物が増えたら除外対象を追加します。

**[.dockerignore](.dockerignore)**

Docker のビルドコンテキストに送らないファイルを指定します。環境変数ファイル、Git の管理情報、一時ファイルなどを除外します。`.gitignore` とは別の設定で、こちらは Docker ビルドに対して働きます。

**[README.md](README.md)**

セットアップ、動作確認、リリース、各ファイルの役割をまとめた手順書です。起動方法や必要な設定を変えたときは、この説明も合わせて更新します。
