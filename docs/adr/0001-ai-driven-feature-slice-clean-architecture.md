# ADR 0001: AI駆動開発向けに Feature Slice 型の軽量 Clean Architecture を採用する

- Status: Accepted
- Date: 2026-05-28
- Scope: Nomo Backend

## Context

Nomo Backend は現在、Go の HTTP API として Supabase/PostgREST を呼び出す構成になっている。
README でも「authenticated requests を Supabase/PostgREST に proxy し、RLS を Supabase 側で効かせる」方針が明記されている。

現状の主な構成は以下。

```text
cmd/api
internal/config
internal/httpapi
internal/supabase
```

この構成は小規模な API では速く実装できる一方で、`internal/httpapi` の handler に以下が集まりやすい。

- HTTP request/response の処理
- 入力 validation
- 業務ルール
- Supabase/PostgREST query の組み立て
- 永続化
- 通知などの副作用

今後、お誘い、友達申請、思い出投稿、通知、管理画面などのルールが増えると、AI/人間のどちらにとっても変更範囲の把握が難しくなりやすい。
特に AI 駆動開発では、巨大な handler や横断的に散らばった暗黙ルールは、意図しない変更や仕様退行の原因になりやすい。

## Decision

Nomo Backend では、今後の通常実装方針として **Feature Slice 型の軽量 Clean Architecture** を採用する。

DDD は全面採用しない。
代わりに、業務ルールが濃い feature だけに必要な domain model / domain function を置く。
単純 CRUD や薄い proxy 処理は過剰に DDD 化しない。

基本方針は以下。

```text
internal/features/<feature>/
  handler.go               # HTTP boundary。薄く保つ
  usecase.go               # 1操作単位のアプリケーション処理
  domain.go                # 業務ルール、状態遷移、値の制約
  repository.go            # usecase から見た永続化 interface / port
  supabase_repository.go   # Supabase/PostgREST 実装
  *_test.go                # domain/usecase を中心に仕様を固定するテスト
```

必要に応じて feature 内をさらに分けてもよいが、最初から細分化しすぎない。

## Why Feature Slice

AI 駆動開発では、横割りのレイヤー構成だけにすると、1つの機能変更で多くのディレクトリを横断しやすい。

例:

```text
internal/domain
internal/application
internal/infrastructure
internal/interfaces
```

この構成は大規模開発では有効だが、Nomo の現在の規模と AI による局所修正を考えると、feature 単位で関連コードが近くにある方が安全に変更しやすい。

Feature Slice では、`invites` の変更ならまず `internal/features/invites` を見ればよい。
AI にも「この feature の範囲だけ変更する」と指示しやすい。

## Why Lightweight Clean Architecture

Clean Architecture の依存方向だけは守る。

```text
HTTP handler / Supabase / FCM などの外側
        ↓
Usecase
        ↓
Domain
```

内側の domain/usecase は、なるべく HTTP、Supabase、FCM、環境変数を直接知らない。
これにより、以下がやりやすくなる。

- handler を薄くする
- Supabase query の文字列変更と業務ルール変更を分離する
- usecase/domain のテストを HTTP なしで書く
- AI が修正対象を間違えにくくする

ただし、Clean Architecture を厳密にやりすぎて不要な interface や DTO 変換を増やすことは避ける。

## Why not Full DDD

Nomo Backend では現時点でフル DDD は採用しない。

理由:

1. 実装速度が落ちる
   - request DTO、usecase input、domain model、repository model、DB row、response DTO の変換が増えやすい。
2. Go では過剰設計になりやすい
   - package や interface が増えすぎると、むしろ見通しが悪くなる。
3. Supabase/RLS と責務が重複しやすい
   - Nomo は Supabase RLS、DB constraint、PostgREST を活用しているため、全ルールを domain に閉じ込める思想とは相性が悪い部分がある。
4. 仕様がまだ変わる段階では aggregate 設計を固定しすぎるリスクがある
5. 「DDD 風のフォルダ」だけが増え、domain がただの struct 置き場になるリスクがある

したがって、フル DDD ではなく、以下の判断を採用する。

- 状態遷移、権限、回数制限、通知発火条件など、壊すと仕様退行になるものは domain/usecase に寄せる
- 単純な取得・更新・proxy は無理に entity/aggregate にしない
- Supabase/RLS は引き続き重要な防御層として扱う

## Consequences

### Benefits

- feature ごとの変更範囲が狭くなる
- handler から業務ルールと Supabase query を追い出せる
- AI に「この feature だけ変更」と指示しやすい
- domain/usecase の table-driven test が仕様書になる
- Supabase/PostgREST の詳細を repository 実装に閉じ込められる
- 将来 feature が複雑化したときに、DDD へ段階的に寄せやすい

### Costs / Trade-offs

- 現状よりファイル数は増える
- 単純な endpoint では少し冗長に見える
- feature 間で共通化したくなる誘惑が増える
- repository interface と Supabase 実装の間に変換が必要になる
- 既存 `internal/httpapi` から段階的に移行する間は、新旧構成が一時的に混在する

### Mitigations

- すべてを一括リライトしない
- まずルールが濃い feature から移す
- 共通 package は急いで作らず、3回以上重複してから検討する
- domain は「業務判断」を置く場所に限定し、単なる JSON struct 置き場にしない
- usecase test を必ず増やし、AI の変更をテストで縛る

## Migration Policy

既存コードを一気に移動しない。
新規実装または大きめの修正が入る feature から、縦に1本ずつ移行する。

優先順位:

1. `invites`
   - 状態、日付、相手の状況、通知が絡むため最優先。
2. `friendrequests`
   - pending/accepted/rejected と friendship 作成が絡む。
3. `memories`
   - 1日1回制限、tagged friend、like/report、official log が絡む。
4. `notifications`
   - 発火条件と push 副作用が絡む。
5. `profiles`
   - 比較的 CRUD 寄りなので後回しでよい。
6. `admin`
   - service role を使うため、通常 user feature とは分離して慎重に扱う。

## Rules for Future Work

- 新しい複雑な feature は `internal/features/<feature>` に作る。
- 既存 feature を修正するとき、handler 内の業務ルールが増えるなら usecase/domain に切り出す。
- handler は以下に限定する。
  - path/query/body/header の読み取り
  - request DTO validation の入り口
  - usecase 呼び出し
  - HTTP status と response 変換
- usecase は 1操作を表す名前にする。
  - `CreateInvite`
  - `AcceptFriendRequest`
  - `CreateMemory`
- domain は業務判断を表す。
  - `CanAcceptInvite`
  - `NewFriendRequest`
  - `ValidateDailyMemoryLimit`
- repository は usecase が必要とする意図を表す method 名にする。
  - `FindActiveInviteForDate`
  - `CreateFriendship`
  - `ListVisibleMemories`
- Supabase の table/column 文字列は原則 `supabase_repository.go` に閉じ込める。
- RLS/DB constraint は引き続き最終防衛線。backend domain check だけを信頼しない。
- AI に依頼する場合は、対象 feature と変更可能ファイル範囲を明確にする。

## Related Document

詳細な実装ガイドは以下を参照する。

- `docs/architecture/ai-driven-feature-slice.md`
