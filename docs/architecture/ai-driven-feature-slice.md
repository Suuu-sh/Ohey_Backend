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
internal/features/invites/
  handler.go
  usecase.go
  domain.go
  repository.go
  supabase_repository.go
  usecase_test.go
```

避けたい例:

```text
internal/domain/planinvite.go
internal/application/create_invite.go
internal/infrastructure/supabase/planinvite_repository.go
internal/interfaces/http/planinvite_handler.go
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

    result, err := h.usecase.CreateInvite(r.Context(), CreateInviteInput{
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

- `CreateInvite`
- `AcceptInvite`
- `RejectInvite`
- `SendFriendRequest`
- `AcceptFriendRequest`
- `CreateMemory`
- `LikeMemory`

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

func CanAcceptInvite(invite Invite, actorUserID string) error {
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
type Invite struct {
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
    FindActiveInviteForDate(ctx context.Context, authToken, fromUserID, toUserID, date string) (*Invite, error)
    CreateInvite(ctx context.Context, authToken string, invite NewInvite) (Invite, error)
    UpdateInviteStatus(ctx context.Context, authToken, inviteID string, status InviteStatus) (Invite, error)
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
    InviteReceived(ctx context.Context, authToken string, invite Invite) error
    InviteAccepted(ctx context.Context, authToken string, invite Invite) error
}
```

usecase では「どのイベントが起きたか」を明示し、通知の保存や push の詳細は notifier 実装に閉じ込める。

ただし初期移行では、既存 `internal/httpapi` の通知関数を adapter として呼ぶ形でもよい。
一気に通知基盤を作り替えない。

## Naming Conventions

### Feature directory

- 小文字、必要なら複数語をつなげる
- 例: `invites`, `friendrequests`, `memories`, `notifications`

### Usecase method

- 動詞 + 対象
- 例: `CreateInvite`, `AcceptFriendRequest`, `CreateMemory`

### Domain function

- 判断は `Can...` / `Validate...`
- 生成は `New...`
- 状態変更は domain method でもよい

例:

```go
func NewFriendRequest(fromUserID, toUserID string) (FriendRequest, error)
func CanAcceptFriendRequest(request FriendRequest, actorUserID string) error
func (i *Invite) Accept(actorUserID string) error
```

### Repository method

- table 操作ではなく usecase の意図で書く
- 例: `FindPendingRequestBetweenUsers`, `CreateFriendship`, `ListVisibleMemories`

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
func TestCreateInviteRejectsSelfInvite(t *testing.T)
func TestCreateMemoryRejectsSecondDailyLog(t *testing.T)
```

### Table-driven test

AI が仕様を把握しやすいので、条件分岐が多い rule は table-driven test にする。

```go
func TestCanAcceptInvite(t *testing.T) {
    tests := []struct {
        name    string
        invite  Invite
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

例: `invites` を移す場合

```text
Before:
internal/httpapi/appdata.go
  createInvite
  updateInvite
  blockedInviteStatusMessage

After:
internal/features/invites/
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

## Current Migration Status

2026-05-28 時点で、以下は `internal/features/<feature>` に domain/usecase/repository を持つ形へ移行済み。

- `invites`
- `memories`
- `friends`
- `notifications`
- `media`
- `profiles`

移行済み feature でも、既存 route との接続のために `internal/httpapi` が薄い adapter として残る場合がある。
この状態は許容するが、新しい業務ルールや Supabase query は `internal/httpapi` に戻さない。

未移行または慎重に扱うもの:

- `admin`
  - service role を使うため、通常 user feature と同じ slice に混ぜない。
  - user 向け profile validation を再利用する場合も、service role 境界は admin 側に残す。

### 1. Invites

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

### 3. Memories

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
- ただし auth user id と公開 `user_id` が混同されると事故が大きいため、user bootstrap / profile update の制約は domain/usecase に集約する

#### Profiles / User Bootstrap の方針

`profiles` は CRUD 寄りだが、認証境界に近いので Feature Slice 化する価値がある。
特に以下の混同を避ける。

- Supabase Auth の UUID: DB の `profiles.id`。常に authenticated user から取る。
- 公開ユーザーID: DB の `profiles.user_id`。ユーザーが入力する handle。

実装方針:

- `internal/features/profiles/domain.go` を profile 入力制約の正本にする。
- `BootstrapProfile` は初回作成/再送用の upsert として扱う。
- bootstrap payload の `id` は request body ではなく authenticated user id から作る。
- `UpdateProfile` は authenticated user id に紐づく profile だけを patch する。
- 通常 user API では caller の Supabase JWT を使い、RLS を最終防衛線にする。
- admin API が同じ制約を使う場合も、service role 境界は admin 側に残す。

現在の profile 制約:

| 項目 | 制約 |
| --- | --- |
| `id` | authenticated user id。UUID 必須。request body から受け取らない |
| `user_id` | 3〜24文字。英数字と `_` のみ。前後空白は trim |
| `display_name` | 1〜40文字。前後空白は trim |
| `gender` | bootstrap 時のみ設定可能。空は `unspecified`。許可値は `unspecified` / `male` / `female` |
| `character_key` | bootstrap では空なら `avatar`。更新時は文字列として trim |
| `avatar_url` | `string` または `null`。前後空白は trim。最大 4096 byte |
| `updated_at` | backend の現在時刻で必ず更新 |

更新制約:

- `gender` の変更は拒否する。
- 更新できる field は `user_id`, `display_name`, `character_key`, `avatar_url` に限定する。
- unknown field は repository payload に入れない。
- validation error は feature error として返し、handler で HTTP status に変換する。

この判断のメリット:

- auth user UUID と公開 `user_id` の取り違えを防げる。
- 初回作成と更新で同じ制約を使える。
- admin など別入口でも同じ validation を参照しやすい。
- profile 周りの事故を usecase/domain test で固定できる。

デメリット:

- CRUD 寄りの処理としてはファイル数が増える。
- `internal/httpapi` の adapter と `internal/features/profiles` が一時的に混在する。
- `gender` など更新不可 field を変更したくなった場合、product decision と migration が必要になる。

今後 profile field を増やす場合は、以下を同じ変更で更新する。

1. `internal/features/profiles/domain.go`
2. `internal/features/profiles/usecase_test.go`
3. `internal/features/profiles/supabase_repository.go`
4. 必要な `internal/httpapi` adapter
5. この docs の制約表

### 6. Admin

理由:

- service role を使う
- 通常 user API と安全境界が違う
- user feature とは別に慎重に移す

## AI Agent 向け作業ルール

AI にこの Backend を変更させる場合、以下を守る。

1. 対象 feature を明確にする
   - 例: `invites` のみ、`friendrequests` のみ。
2. 変更可能ファイル範囲を狭くする
   - 例: `internal/features/invites/**` と必要な routing のみ。
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

## Example: Invite の分割イメージ

```text
internal/features/invites/
  domain.go
    InviteStatus
    Invite
    NewInvite
    CanCreateInvite
    CanUpdateInviteStatus

  repository.go
    Repository interface
    Notifier interface

  usecase.go
    CreateInvite
    UpdateInviteStatus

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

## 2026-05-28 追加方針: Home Feed / Domain Event / Moderation / Friend Groups

### Home Feed endpoint

Home の表示判断は今後 `GET /v1/home/feed` を優先する。
既存 `GET /v1/memories` はカレンダーやプロフィールなどの raw memory 一覧として残し、Home 専用の整形は `internal/features/homefeed` に閉じ込める。

Backend が返す feed row には従来の memory fields に加えて `feed_item` を付ける。
`feed_item` は以下を持つ。

- `post_kind`: `mine` / `friend` / `official`
- `author_name`
- `displayable`
- `owned_by_me`
- `can_like` / `can_report` / `can_delete`
- `like_count` / `liked_by_me`
- `photo_path` / `caption_y` / `link_url`
- `accent_seed` / `tilt` / `prop`

判断理由:

- Mobile が friend list を別途見て feed 表示可否を再判定しなくてよくなる。
- Home UI のカード表現を変える時、Backend の `feed_item` contract を見ればよい。
- 将来ランキング、レコメンド、非表示、広告/公式投稿の差し込みを `homefeed` usecase に集約できる。

Trade-off:

- `memories` と `homefeed` で似た Supabase query が一部重複する。
- `feed_item` が UI 寄りになりすぎると Backend が presentation 詳細を持ちすぎる。

対策:

- Backend は色そのものや layout pixel ではなく、`post_kind`, `accent_seed`, `can_report` のような意味/判断を返す。
- raw `memories` contract は残し、Home 専用整形は `/v1/home/feed` に限定する。

### invites domain event

`invites` では、invite 作成・承認時に notification を直接呼ばない。
usecase は `DomainEvent` を発行し、HTTP adapter が event を notification usecase へ橋渡しする。

現在の event:

- `invite.created`
- `invite.accepted`

判断理由:

- push 以外に analytics / reminder / activity log を後から足しやすい。
- usecase の責務が「招待の状態変更」と「イベント発行」までに絞られる。
- notification の保存・push 送信は subscriber 側に閉じ込められる。

Trade-off:

- 初期実装では in-process event なので、永続 event queue ではない。
- notification 失敗時の retry はまだ保証しない。

今後 event の信頼性が必要になったら、outbox table か job queue を追加する。

### Report / Moderation

通報は `memory_reports` を以下の2つの意味で扱う。

1. moderation queue item
2. reporter にとっての feed 非表示 signal

Backend 方針:

- report reason は domain で許可値に正規化する。
- 同じ user が同じ memory を通報した場合は duplicate として扱い、追加 insert しない。
- 自分の投稿は通報できない。
- `homefeed` / `memories` は reporter が通報済みの memory を除外する。
- admin は `GET /v1/admin/memory-reports` で確認し、migration 適用後は `PATCH /v1/admin/memory-reports/{id}` で `status` を更新する。

現在の allowed reason:

- `spam`
- `harassment`
- `inappropriate`
- `violence`
- `minor_safety`
- `other`

現在の moderation status:

- `pending`
- `reviewing`
- `resolved`
- `dismissed`

Trade-off:

- report 作成と非表示が同じ table に載るため、将来「通報せず非表示だけ」が必要になったら別 table が必要になる。
- admin moderation columns は migration 適用前の環境では更新できない。

### Custom Friend Groups

Custom friend groups は Backend API を追加し、Mobile は Backend sync を優先しつつ、migration rollout 中は local cache に fallback する。

Endpoint:

- `GET /v1/friend-groups`
- `PUT /v1/friend-groups`

Backend model:

- `friend_groups`
- `friend_group_members`

判断理由:

- 機種変更・複数端末で group が同期できる。
- group member は既存 friendship であることを usecase と RLS の両方で確認する。
- `client_id` を持たせ、Mobile 既存 local id と互換にする。
- 正規化 table にしておくことで、将来の共有 group に拡張しやすい。

Trade-off:

- local-only より保存経路が増える。
- migration 未適用時は Backend sync が no-op/fallback になる。
- snapshot 保存は複数 PostgREST call で構成され、完全な transaction ではない。

今後、グループ共有・権限・招待制が必要になったら、`friend_groups.owner_user_id` に加えて membership/role table を追加する。

## 2026-05-28 追加方針: Daily Status / Feed Ranking / Safety / Media Lifecycle

### Daily Status Feature Slice

`daily_statuses` は `internal/features/dailystatuses` に切り出す。
HTTP handler は以下だけを担当し、date / status / owner 制約は feature 側へ寄せる。

Endpoint:

- `GET /v1/daily-status?date=YYYY-MM-DD`
- `PUT /v1/daily-status`
- `GET /v1/daily-statuses/month?month=YYYY-MM`

判断理由:

- `status` の許可値、`status_date` の `YYYY-MM-DD` validation、owner user id の固定を usecase test で守れる。
- Calendar は月表示のたびに日別 API を大量に呼ばず、月次取得へ寄せられる。
- Friends の空き状況参照は `friends` feature が日付を決めて読むだけにし、Mobile から任意 user の daily status を直接読む設計にしない。

Trade-off:

- `daily_statuses` 自体は Supabase/RLS の owner policy に依存するため、Backend だけで全権限を表現しているわけではない。
- Calendar の隣月セルは月次 prefetch の対象外にし、必要な場合だけ日別 endpoint で補完する。

### Home Feed pagination / ranking

`GET /v1/home/feed` は以下の query を受け付ける。

- `limit`: default 50 / max 100
- `cursor`: `feed_cursor` または legacy `sort_at` RFC3339

Backend は各 row / `feed_item` に以下を追加する。

- `rank_score`
- `feed_rank_score`
- `feed_cursor`

現在の ranking は「recency を主軸に、同秒などの近い投稿で official / mine / friend の weight を足す」軽量実装にする。
つまり古い公式投稿を常に上に固定するのではなく、投稿時刻を壊さず将来の ranking 差し替えポイントを作る。

判断理由:

- 投稿数が増えても Mobile は `limit/cursor` で段階取得できる。
- ranking の責務を Mobile ではなく `homefeed` usecase に寄せることで、official post / friend post / 自分 post の順序調整を Backend だけで変えられる。
- `rank_score` と `feed_cursor` を先に contract に入れておくと、将来おすすめ・ランキング・広告差し込みを envelope 変更なしで試しやすい。

Trade-off:

- 現在の response は互換性維持のため array のままなので、`next_cursor` を top-level で返せない。
- cursor は各 row の `feed_cursor` を次回 request に渡す設計にしている。
- ranking score は絶対値ではなく Backend 内部の並び制御用なので、Mobile で意味を解釈しすぎない。

### Notification domain event expansion / outbox foundation

通知を発生させる usecase は、notification usecase を直接呼ばず、Domain Event を publish する方針へ寄せる。

現在の event:

- `invite.created`
- `invite.accepted`
- `friend_request.created`
- `friend_request.accepted`
- `memory.tagged`
- `memory.liked`
- `memory.reported`
- `system_notification.created`

HTTP adapter が event を受け取り、既存 notification usecase へ橋渡しする。
同時に `notification_outbox` へ event payload を保存する。

判断理由:

- push / analytics / reminder / audit log を subscriber として横に増やせる。
- friend request や memory usecase は「状態変更 + event 発行」までに責務を絞れる。
- `notification_outbox` に event が残るため、公開後に retry worker / cron を追加しやすい。

Trade-off:

- 現時点では in-process subscriber で処理し、outbox row は `processed` audit として保存する。
- 本当の retry 保証には、pending row を処理する worker と idempotency key が必要。
- notification usecase が push 失敗を warn に落とす設計なので、push 単位の retry は次段階で分離する。

次に retry を強める時は、`notification_outbox.status = pending` を worker が処理し、notification 作成と push 送信の結果で `processed` / `failed` / `next_attempt_at` を更新する。

### Block / Mute / Hide User Safety

Backend API:

- `POST /v1/user-blocks`
- `DELETE /v1/user-blocks/{user_id}`
- `POST /v1/user-mutes`
- `DELETE /v1/user-mutes/{user_id}`
- `POST /v1/memory-hides`
- `DELETE /v1/memory-hides/{memory_id}`

Backend model:

- `user_blocks`
- `user_mutes`
- `memory_hides`

Feed 側の方針:

- `homefeed` / `memories` は、自分が report 済みまたは hide 済みの memory を除外する。
- 自分が block / mute した user の通常投稿は ranking 前に除外する。
- official post は運営投稿として扱い、owner が一致しても block/mute による除外対象にしない。
- friend request / invite 作成時は block 関係があれば拒否する。

判断理由:

- 通報だけに頼らず、ユーザー本人が即時に見たくない投稿・相手を消せる。
- ranking の前段で除外するため、hidden / blocked user の投稿が pagination の枠を消費しにくい。
- `report` と `hide` を分けたので、「運営へ通報しないが自分の feed から消したい」導線を後から UI に足せる。

Trade-off:

- block / mute / hide の UI はまだ Mobile 側で未接続。Backend contract を先に用意した段階。
- block 関係の完全な相互拒否には、accept/update 系にも追加の事前チェックが必要。
- migration rollout 中に table が未作成でも既存 feed が落ちないよう、optional safety table は missing 時 no-op fallback にしている。

### Media Lifecycle

memory 削除時、返却 row に `photo_path` がある場合は Supabase Storage の `nomo-photos` object 削除を試みる。

判断理由:

- 投稿削除後に orphan photo が残り続けることを減らし、Storage cost と privacy risk を下げる。
- 削除済み DB row に対する cleanup failure で UX を壊さないよう、Storage delete は best-effort にする。

Trade-off:

- Storage delete が失敗しても API response は成功する。失敗は logger warn で観測する。
- orphan object の定期検出、signed display URL の Backend 統一、report/hidden 時の photo visibility 制御は次段階。

## 2026-05-28 追加実装: Mobile Safety UI / Outbox Retry / API Contract

### Mobile safety UI connection

Home feed の投稿メニューから以下を呼び出す。

- `hide`: `POST /v1/memory-hides`
- `mute`: `POST /v1/user-mutes`
- `block`: `POST /v1/user-blocks`
- `report`: `POST /v1/memories/{id}/report`

判断理由:

- 通報だけでなく、ユーザー本人が即時に feed を整えられる。
- hide は投稿単位、mute/block は user 単位に分け、重さの違う操作を混同しない。
- block は destructive action として確認 sheet を挟む。

Trade-off:

- mute/block 解除 UI はまだ未接続。Backend endpoint はあるため、settings / profile menu から後で足す。
- 投稿メニューに safety action が増えるため、将来は「安全」サブメニューに分ける余地がある。

### Feed pagination mobile connection

Mobile `HomeFeedController` は初回 `limit=20` で取得し、PageView の末尾近くで `feed_cursor` を使って追加取得する。

判断理由:

- 投稿数増加時に初回 Home 表示の payload を抑えられる。
- Backend ranking / cursor contract を Mobile に接続し、将来の ranking 変更に耐えやすくする。

Trade-off:

- response envelope は互換性維持のため array のまま。`next_cursor` は最後の row の `feed_cursor` から読む。
- PageView は index が進んだ時だけ load more するため、極端に短い feed では次回操作まで追加取得しない場合がある。

### Notification outbox retry

Domain event は `notification_outbox` に `pending` として保存し、in-process dispatch の結果で `processed` / `failed` に更新する。
失敗時は `last_error` と exponential backoff 用の `next_attempt_at` を保存する。

追加した retry entrypoint:

- `GET /v1/admin/notification-outbox`
- `POST /v1/admin/notification-outbox/process`
- `/nomo-notification-worker` binary
- Render cron は本番未使用

判断理由:

- event 作成と notification/push dispatch の間で失敗しても、failed row を後で再処理できる。
- admin endpoint で状態を見られるので、公開後の調査がしやすい。
- worker binary は web server と同じ image に残し、将来 cron 化するときの code path を揃える。
- Render cron は課金対象のため、ユーザー数が増えて Pro / paid cron が必要になるまで production では作成しない。

Trade-off:

- notification row 作成後の push 失敗は `last_error` に残るが、既存 unique constraint により retry 時に in-app notification は重複しない。push 再送の完全保証は token 単位 outbox を追加する次段階。
- cron なし期間は failed/pending outbox の自動再送は保証しない。必要時は `POST /v1/admin/notification-outbox/process` を手動実行する。
- 将来 Render cron を有効化する場合は production の `SUPABASE_SERVICE_ROLE_KEY` と `FCM_SERVICE_ACCOUNT_JSON` 設定が必須。

### Block cleanup

`POST /v1/user-blocks` 成功時、Backend は service role があれば以下を整理する。

- 既存 friendship を削除
- 自分が送った pending friend request は `cancelled`
- 相手から来た pending friend request は `rejected`
- 自分が送った pending invite は `cancelled`
- 相手から来た pending invite は `rejected`

判断理由:

- block 後も pending 状態が残ると通知・招待・関係表示で事故が起きる。
- status update にして履歴を残し、物理削除より運用調査しやすくする。

Trade-off:

- block 解除しても friendship / request / invite は自動復元しない。
- service role 未設定環境では cleanup は no-op になり、RLS fallback に依存する。

### Backend-signed display URL

Mobile は raw `photo_path` を受け取ったら `POST /v1/media/display-url` を優先して signed display URL を取得する。
失敗した場合のみ従来の Supabase client signing に fallback する。

判断理由:

- Storage 表示 URL の生成責務を Backend に寄せ、将来 visibility / report / hidden との整合性を取りやすくする。
- Mobile に Storage policy の詳細を持たせにくくする。

Trade-off:

- feed item ごとに display-url call が増える。将来は feed endpoint 側で signed URL を batch 付与する方が効率的。
- 現段階では path validation は memory photo path 形式の検証まで。閲覧権限は feed visibility と RLS に依存する。
- orphan object は `GET /v1/admin/media/orphan-memory-photos?user_id=<uuid>` で user prefix 単位に検出する。実削除は手動確認後に別途行う。

### API contract doc

API contract は `/docs/api/backend-api-contract.md` に分離した。
今後 endpoint / request / response を変える時は、実装と同じ commit でこの contract を更新する。

## 2026-05-28 Operational hardening follow-up

### Supabase migration / RLS verification

Added a static Supabase migration contract verifier in Mobile (`scripts/verify_supabase_rls_contract.py`).

判断理由:

- Nomo の Supabase migrations は Mobile repository 側で管理され、GitHub Actions が dev / production に順次適用する。
- 今回は production 適用を行わず、migration file と RLS contract の破れを先に検出できるようにした。
- `friend_groups`, `friend_group_members`, `user_blocks`, `user_mutes`, `memory_hides`, `notification_outbox`, `memory_reports`, `push_tokens` を重点確認する。

Trade-off:

- static verifier は実 DB への適用成功を完全保証しない。DB 実適用は GitHub Actions / Supabase migration workflow の責務。
- Supabase CLI がローカルに無い場合でも確認できる一方、Postgres の全 SQL syntax check ではない。

### Rate limit / abuse control

Authenticated write endpoints に user/action 単位の in-memory fixed-window limiter を追加した。

対象:

- report 連打
- invite 連打
- friend request 連打
- upload-url 連打
- block / mute 連打

判断理由:

- 公開直後の悪用・誤タップ・簡易 DoS を低コストに抑える。
- 現状の Render は単一 instance 前提なので、まずは DB schema を増やさない in-memory で十分。
- `429` + `Retry-After` を返すことで Mobile / Admin が後から UX を整えやすい。

Trade-off:

- instance restart で bucket はリセットされる。
- 複数 instance 化したら per-instance limit になるため、Redis / DB backed limiter に置き換える。
- 厳密な abuse 防止ではなく、初期公開向けの摩擦追加。

### Push token lifecycle

Mobile / Backend の push token lifecycle を整理した。

- Mobile startup/login: `PUT /v1/me/push-token`
- Firebase token refresh: new token 登録 + previous token best-effort 削除
- logout: current token best-effort 削除
- FCM invalid token response: Backend が `push_tokens` から token を削除

判断理由:

- 端末変更・再インストール・ログアウト後の古い token に push し続ける事故を減らす。
- FCM の token-specific failure はユーザー操作なしで cleanup する方が運用負荷が低い。

Trade-off:

- logout cleanup は best-effort。オフライン/logout 失敗時は残る可能性がある。
- FCM auth/config 系 failure は token 問題ではないため削除しない。outbox / logs で調査する。

### Admin moderation UI minimum

Mobile Admin に pending report queue を追加し、`reviewing` / `resolved` / `dismissed` へ更新できるようにした。

判断理由:

- 公開後、通報が DB に溜まるだけだと運用できない。
- 最小 UI で状態遷移だけ先に入れておくと、後から詳細確認・非表示・削除 action を追加しやすい。

Trade-off:

- 現時点では詳細 evidence / image preview / bulk action はない。
- `moderation_note` は固定文言。必要になったら編集欄を追加する。

### Manual notification outbox operations

Render cron は課金対象のため production 未使用とし、manual runbook を追加した。

判断理由:

- ユーザー数が少ない段階で paid cron を常時動かさない。
- ただし failed/pending outbox を手動で処理できるように operational docs を固定する。

Trade-off:

- cron なし期間は自動再送保証がない。
- 通知信頼性が要求される規模になったら Render cron / external scheduler / DB-backed worker を再検討する。
