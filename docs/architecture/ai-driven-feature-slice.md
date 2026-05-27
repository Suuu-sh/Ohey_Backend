# AI駆動開発向け Backend 設計ガイド

この文書は、Nomo Backend を今後 **AI駆動開発で安全に変更しやすい構造** に寄せるための実装ガイドである。

採用判断の概要は `docs/adr/0001-ai-driven-feature-slice-clean-architecture.md` を参照する。

## 結論

Nomo Backend は今後、**Feature Slice 型の軽量 Clean Architecture** を基本方針にする。

```text
internal/features/<feature>/
  handler.go
  usecase.go
  domain.go
  repository.go
  supabase_repository.go
  *_test.go
```

ただし、フル DDD は採用しない。
業務ルールが濃い feature だけを domain model / domain function 化し、単純 CRUD は薄く保つ。

## 目的

この設計の目的は以下。

1. AI が変更範囲を読み間違えにくくする
2. handler の巨大化を防ぐ
3. 業務ルール、HTTP、Supabase query、通知副作用を分離する
4. usecase/domain test を仕様書として残す
5. Supabase/RLS を活かしつつ、backend 側のルールも見える場所に集める

## 現状の課題

現状の Backend は、主に以下の package で構成されている。

```text
cmd/api
internal/config
internal/httpapi
internal/supabase
```

`internal/httpapi` には API handler、request/response 型、validation、Supabase 呼び出し、通知作成などがまとまっている。
小規模では速いが、以下の問題が出やすい。

- 1つの handler が長くなり、AI が副作用を見落としやすい
- Supabase query 文字列が handler に散らばる
- 業務ルールと HTTP status の判断が混在する
- 通知や push の副作用が主処理と密結合になる
- テストが HTTP handler 経由に偏り、業務ルール単体の仕様が残りにくい

## 設計原則

### 1. Feature 単位に閉じる

横断レイヤーだけで分けず、まず feature 単位で関連コードを近くに置く。

良い例:

```text
internal/features/drinkinvites/
  handler.go
  usecase.go
  domain.go
  repository.go
  supabase_repository.go
  usecase_test.go
```

避けたい例:

```text
internal/domain/drinkinvite.go
internal/application/create_drink_invite.go
internal/infrastructure/supabase/drinkinvite_repository.go
internal/interfaces/http/drinkinvite_handler.go
```

後者は理論上きれいだが、Nomo の規模と AI 駆動開発では変更箇所が遠くなりやすい。

### 2. Handler は薄くする

handler の責務は HTTP boundary に限定する。

やってよいこと:

- path/query/header/body を読む
- JSON decode する
- request DTO を最低限 normalize/validate する
- auth user id / token を usecase input に渡す
- usecase を呼ぶ
- error を HTTP status に変換する
- response DTO を返す

避けること:

- Supabase query を直接組む
- 複雑な状態遷移を判断する
- 通知発火条件を直接判断する
- 複数 table への書き込み手順を handler に書く

目安:

```go
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
    input, ok := decodeCreateRequest(w, r)
    if !ok {
        return
    }

    result, err := h.usecase.CreateDrinkInvite(r.Context(), CreateDrinkInviteInput{
        AuthToken:   bearerToken(r),
        FromUserID:  nomoUserID(r),
        ToUserID:    input.ToUserID,
        InviteDate:  input.InviteDate,
    })
    if err != nil {
        writeFeatureError(w, err)
        return
    }

    writeJSON(w, http.StatusCreated, result)
}
```

### 3. Usecase は 1操作を表す

usecase は application の操作単位を表す。

例:

- `CreateDrinkInvite`
- `AcceptDrinkInvite`
- `RejectDrinkInvite`
- `SendFriendRequest`
- `AcceptFriendRequest`
- `CreateDrinkLog`
- `LikeDrinkLog`

usecase の責務:

- repository から必要な情報を取得する
- domain function/model に判断させる
- 必要な永続化を行う
- 通知などの副作用を明示的に呼ぶ
- handler に返す結果を作る

usecase が直接知ってよいもの:

- domain type/function
- repository interface
- clock interface が必要な場合の現在時刻
- notification sender などの port

usecase が直接知るべきでないもの:

- `net/http`
- PostgREST query string
- Supabase table/column の詳細
- FCM の HTTP API 詳細
- 環境変数

### 4. Domain は業務判断だけ置く

domain は「Nomo のルール」を置く場所。
単なる JSON DTO や DB row の置き場にしない。

良い例:

```go
type InviteStatus string

const (
    InviteStatusPending  InviteStatus = "pending"
    InviteStatusAccepted InviteStatus = "accepted"
    InviteStatusRejected InviteStatus = "rejected"
)

func CanAcceptInvite(invite DrinkInvite, actorUserID string) error {
    if invite.ToUserID != actorUserID {
        return ErrOnlyRecipientCanAcceptInvite
    }
    if invite.Status != InviteStatusPending {
        return ErrInviteAlreadyResolved
    }
    return nil
}
```

避けたい例:

```go
type DrinkInvite struct {
    ID string `json:"id"`
    // ただの JSON struct で、ルールが何もない
}
```

domain に置くべきもの:

- 状態遷移
- 自分自身への操作禁止
- 日付/回数制限
- actor が実行可能かの判定
- 通知を発火すべき条件の判定
- 値オブジェクト化した方が事故を減らせるもの

置きすぎないもの:

- HTTP status
- Supabase query
- JSON tag だらけの response DTO
- FCM payload の詳細

### 5. Repository は usecase 目線の port にする

repository interface は table 操作名ではなく、usecase が必要とする意図で命名する。

良い例:

```go
type Repository interface {
    FindActiveInviteForDate(ctx context.Context, authToken, fromUserID, toUserID, date string) (*DrinkInvite, error)
    CreateInvite(ctx context.Context, authToken string, invite NewDrinkInvite) (DrinkInvite, error)
    UpdateInviteStatus(ctx context.Context, authToken, inviteID string, status InviteStatus) (DrinkInvite, error)
}
```

避けたい例:

```go
type Repository interface {
    Get(ctx context.Context, table string, query url.Values, out any) error
    Upsert(ctx context.Context, table string, body any, out any) error
}
```

Supabase/PostgREST 固有の table/column/query 文字列は `supabase_repository.go` に閉じ込める。

## Feature の標準ファイル

### `handler.go`

HTTP boundary。
既存 `internal/httpapi` との接続が必要な間は、薄い adapter として扱う。

含めるもの:

- route registration helper
- handler struct
- request DTO
- response DTO
- HTTP error mapping

含めないもの:

- PostgREST query
- 複雑な業務ルール
- FCM payload 組み立て詳細

### `usecase.go`

feature の application flow。
1操作につき関数または method を作る。

含めるもの:

- input/output type
- usecase struct
- operation method
- transaction 的な手順
- repository/notification port の呼び出し

注意:

- 長くなったら、private method で意図ごとに分ける。
- DB row 変換は repository 側に置く。

### `domain.go`

業務ルール。
テストしやすい pure function を優先する。

含めるもの:

- domain type
- status enum
- constructor
- validation
- state transition
- domain error

注意:

- domain が外部 API を直接呼ばない。
- ルールがない型は無理に domain に置かない。

### `repository.go`

usecase から見た port/interface。

含めるもの:

- repository interface
- notification port など外部副作用の interface
- usecase test 用 fake を作りやすい method 設計

注意:

- interface を細かくしすぎない。
- 実装が1つでも、usecase test のために必要なら置く。
- 単純すぎる feature では repository interface を省略してもよいが、Supabase query が handler に戻らないようにする。

### `supabase_repository.go`

Supabase/PostgREST 実装。

含めるもの:

- table/column/select 文字列
- PostgREST query 組み立て
- DB row struct/map との変換
- Supabase API error の扱い

注意:

- service role を使う処理は明示的に分ける。
- 通常 user feature では caller の Supabase JWT と RLS を前提にする。
- RLS を迂回する処理は admin feature に閉じ込める。

### `*_test.go`

AI 駆動開発ではテストが仕様書になる。
特に domain/usecase の table-driven test を優先する。

優先してテストするもの:

- 状態遷移
- 権限エラー
- 自分自身への操作禁止
- 重複作成防止
- 日付制約
- 通知が発火する/しない条件
- repository 呼び出し順序の重要な部分

## Error Handling

feature 内では、HTTP status ではなく意味のある error を返す。

例:

```go
var (
    ErrInviteNotFound              = errors.New("invite not found")
    ErrOnlyRecipientCanAcceptInvite = errors.New("only recipient can accept invite")
    ErrInviteAlreadyResolved        = errors.New("invite already resolved")
)
```

handler で HTTP status に変換する。

```go
switch {
case errors.Is(err, ErrInviteNotFound):
    writeError(w, http.StatusNotFound, "invite not found")
case errors.Is(err, ErrOnlyRecipientCanAcceptInvite):
    writeError(w, http.StatusForbidden, "only recipient can accept invite")
default:
    writeError(w, http.StatusInternalServerError, "internal server error")
}
```

Supabase の raw error body は client に返さない。
既存の upstream error masking 方針を維持する。

## Supabase / RLS との関係

Nomo Backend は Supabase RLS を重要な安全境界として使う。
Feature Slice 化してもこの方針は変えない。

方針:

- 通常 user API は caller の Supabase JWT を使う
- RLS/DB constraint は最終防衛線
- backend domain/usecase は UX と仕様の明示化のために事前チェックする
- service role は admin や backend-only 処理に限定する
- domain check と DB constraint が重複してもよい。役割が違うため。

役割分担:

```text
Domain/Usecase:
  ユーザーに分かりやすいエラー、仕様の可読性、テスト可能性

Supabase RLS/Constraint:
  認可・整合性の最終防衛線

Supabase Repository:
  PostgREST query と row 変換
```

## Notification / Push の扱い

通知は副作用なので、主処理に埋め込みすぎない。

推奨:

```go
type Notifier interface {
    DrinkInviteReceived(ctx context.Context, authToken string, invite DrinkInvite) error
    DrinkInviteAccepted(ctx context.Context, authToken string, invite DrinkInvite) error
}
```

usecase では「どのイベントが起きたか」を明示し、通知の保存や push の詳細は notifier 実装に閉じ込める。

ただし初期移行では、既存 `internal/httpapi` の通知関数を adapter として呼ぶ形でもよい。
一気に通知基盤を作り替えない。

## Naming Conventions

### Feature directory

- 小文字、必要なら複数語をつなげる
- 例: `drinkinvites`, `friendrequests`, `drinklogs`, `notifications`

### Usecase method

- 動詞 + 対象
- 例: `CreateDrinkInvite`, `AcceptFriendRequest`, `CreateDrinkLog`

### Domain function

- 判断は `Can...` / `Validate...`
- 生成は `New...`
- 状態変更は domain method でもよい

例:

```go
func NewFriendRequest(fromUserID, toUserID string) (FriendRequest, error)
func CanAcceptFriendRequest(request FriendRequest, actorUserID string) error
func (i *DrinkInvite) Accept(actorUserID string) error
```

### Repository method

- table 操作ではなく usecase の意図で書く
- 例: `FindPendingRequestBetweenUsers`, `CreateFriendship`, `ListVisibleDrinkLogs`

## Testing Policy

### 優先順位

1. domain の pure function test
2. usecase の fake repository test
3. Supabase repository の query 組み立て test
4. HTTP handler test

既存の `router_handler_test.go` は残しつつ、新規 feature は usecase/domain test を増やす。

### テスト名

仕様が読める名前にする。

```go
func TestCanAcceptInviteRejectsNonRecipient(t *testing.T)
func TestCreateDrinkInviteRejectsSelfInvite(t *testing.T)
func TestCreateDrinkLogRejectsSecondDailyLog(t *testing.T)
```

### Table-driven test

AI が仕様を把握しやすいので、条件分岐が多い rule は table-driven test にする。

```go
func TestCanAcceptInvite(t *testing.T) {
    tests := []struct {
        name    string
        invite  DrinkInvite
        actorID string
        wantErr error
    }{
        // ...
    }
}
```

## Migration Guide

### Step 1: 新規 feature から採用

新しい API または大きな仕様追加は `internal/features/<feature>` で作る。
既存 `internal/httpapi` に直接巨大な handler を追加しない。

### Step 2: 既存 feature は修正時に縦に切り出す

既存の `internal/httpapi` を一括移動しない。
作業対象になった feature から、handler/usecase/domain/repository を縦に分ける。

例: `drink-invites` を移す場合

```text
Before:
internal/httpapi/appdata.go
  createDrinkInvite
  updateDrinkInvite
  blockedInviteStatusMessage

After:
internal/features/drinkinvites/
  handler.go
  usecase.go
  domain.go
  repository.go
  supabase_repository.go
  usecase_test.go
```

### Step 3: ルート登録を徐々に差し替える

最初は `internal/httpapi/router.go` から feature handler を呼ぶ adapter でもよい。
最終的には feature handler が route registration を持つ形に寄せる。

### Step 4: 古い helper の共通化は急がない

共通化の基準:

- 3つ以上の feature で同じ責務が出た
- 名前が明確
- 依存方向を壊さない
- AI が見ても使い方を間違えにくい

安易な `internal/common` や `internal/utils` は避ける。

## Migration Priority

### 1. Drink Invites

理由:

- pending/accepted/rejected など状態がある
- invite date が絡む
- 相手の daily status による制約がある
- 通知発火がある

最初の移行候補。

### 2. Friend Requests

理由:

- 自分自身への申請禁止
- pending の重複防止
- accept 時に friendship 作成がある
- 通知発火がある

### 3. Drink Logs

理由:

- 1日1回制限
- tagged friend
- like/report
- official log
- feed visibility

### 4. Notifications

理由:

- insert notification と push が絡む
- 失敗時の扱いが主処理と異なる

### 5. Profiles

理由:

- 比較的 CRUD 寄り
- 最初から重く分ける価値は低い

### 6. Admin

理由:

- service role を使う
- 通常 user API と安全境界が違う
- user feature とは別に慎重に移す

## AI Agent 向け作業ルール

AI にこの Backend を変更させる場合、以下を守る。

1. 対象 feature を明確にする
   - 例: `drinkinvites` のみ、`friendrequests` のみ。
2. 変更可能ファイル範囲を狭くする
   - 例: `internal/features/drinkinvites/**` と必要な routing のみ。
3. 既存の Supabase/RLS 前提を壊さない
   - caller JWT を使う通常 API を service role に変えない。
4. handler に新しい業務ルールを足さない
   - domain/usecase に追加する。
5. 仕様変更には test を追加する
   - 特に domain/usecase test。
6. raw upstream error を client に返さない
7. `internal/common` を安易に作らない
8. unrelated feature の挙動をついでに変更しない
9. 既存の API response 形式を変える場合は明示的な理由を docs または PR 説明に残す
10. 迷ったら「薄い handler、意図が見える usecase、ルールが見える domain」を優先する

## メリット

- AI が読むべき範囲が feature 単位で狭くなる
- 人間も仕様を探しやすい
- handler の肥大化を防げる
- Supabase/PostgREST の文字列 query を repository に閉じ込められる
- domain/usecase test が仕様書として機能する
- ルールが増えても段階的に DDD に寄せられる
- フル DDD より実装速度を落としにくい

## デメリット

- 現状よりファイル数が増える
- 小さい変更でも usecase/repository を触ることがある
- 初期移行中は `internal/httpapi` と `internal/features` が混在する
- feature 間で似た型や helper が一時的に重複する
- repository interface の設計が雑だと、結局 Supabase 依存が漏れる

## デメリットへの対策

- 小さい CRUD は無理に full set のファイルを作らない
- ルールがある feature から移す
- 共通化は3回目まで待つ
- interface は usecase が必要とする最小 method にする
- 移行中の混在は許容し、移行した feature から docs/test を厚くする

## 判断基準

### Feature Slice 化するべき

- 状態遷移がある
- 複数 table を更新する
- 通知や push が絡む
- 認可や actor 判定が複雑
- 日付や回数制限がある
- テストで固定したい業務ルールがある

### 既存 handler のままでもよい

- 単純な health check
- 単純な read-only proxy
- 仕様ルールがほぼない取得 API
- 近いうちに変更予定がない薄い処理

## Example: Drink Invite の分割イメージ

```text
internal/features/drinkinvites/
  domain.go
    InviteStatus
    DrinkInvite
    NewInvite
    CanCreateInvite
    CanUpdateInviteStatus

  repository.go
    Repository interface
    Notifier interface

  usecase.go
    CreateDrinkInvite
    UpdateDrinkInviteStatus

  supabase_repository.go
    SupabaseRepository
    PostgREST query
    row mapping

  handler.go
    Create handler
    Update handler
    request/response DTO

  usecase_test.go
    self invite rejection
    duplicate active invite rejection
    accepted invite notification
```

## Final Guideline

今後の Nomo Backend では、以下を標準判断にする。

> 複雑な feature は Feature Slice 型の軽量 Clean Architecture に寄せる。  
> ただしフル DDD は目的化しない。  
> 業務ルールが増える場所だけ domain/usecase/test に集め、Supabase/RLS の強みは維持する。
