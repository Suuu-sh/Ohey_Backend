# Nomo Backend API Contract

Last updated: 2026-05-28

この document は Mobile / Backend / AI agent が同じ contract を参照するための要約です。Backend は Feature Slice 型の軽量 Clean Architecture を基本にし、以下の endpoint を優先 contract として扱います。

## Auth

通常 endpoint は以下の headers が必須です。

- `Authorization: Bearer <supabase access token>`
- `X-Nomo-User-ID: <auth.users.id>`

Admin endpoint は backend 側で `NOMO_ADMIN_EMAILS` に一致する Supabase Auth user のみ許可します。

### `DELETE /v1/me/account`

ログイン中の本人アカウントを削除する。Backend が caller JWT と `X-Nomo-User-ID` の一致を検証した後、trusted server 側の Supabase service role で Auth user を削除する。Mobile は呼び出し前に push token unregister を best-effort で実行し、成功後は local session を破棄する。

## Rate limit / Abuse control

Authenticated write endpoints return `429 Too Many Requests` with `Retry-After` when the per-user limit is exceeded.

Current in-memory limits:

- `POST /v1/drink-logs/{id}/report`: 10 / hour
- `POST /v1/drink-invites`: 20 / hour
- `POST /v1/friend-requests`: 20 / hour
- `POST /v1/media/upload-url`: 30 / hour
- `POST /v1/user-blocks`: 30 / hour
- `POST /v1/user-mutes`: 60 / hour

Render single instance 前提の軽量 limiter。複数 instance / 大規模化時は Redis / DB backed limiter に置き換える。

## Home Feed

### `GET /v1/home/feed`

Query:

- `limit`: positive integer。default `50`、max `100`。
- `cursor`: 前回 response の最後の row の `feed_cursor`。旧 `sort_at` RFC3339 も互換で受け付ける。

Response: drink log row の array。各 row は raw fields に加えて以下を持ちます。

- `feed_item`
- `feed_post_kind`
- `feed_displayable`
- `feed_author_name`
- `feed_owned_by_me`
- `feed_can_report`
- `feed_can_delete`
- `rank_score`
- `feed_rank_score`
- `feed_cursor`

方針:

- Mobile は Home 表示では `/v1/drink-logs` ではなく `/v1/home/feed` を使う。
- reported / hidden / blocked / muted user は ranking 前に除外する。
- `rank_score` は Backend 内部の並び制御用。Mobile は表示ロジックとして解釈しない。

## Daily Status

### `GET /v1/daily-status?date=YYYY-MM-DD`

自分の指定日 status を返す。`date` 省略時は Backend の today。

### `PUT /v1/daily-status`

Body:

```json
{"status_date":"2026-05-28","status":"can_drink_today"}
```

Allowed status:

- `unselected`
- `can_drink_today`
- `non_alcohol`
- `liver_rest`
- `has_plans`

### `GET /v1/daily-statuses/month?month=YYYY-MM`

Calendar 用。自分の月次 status rows を返す。

## Friends

### `GET /v1/friends`

自分の friendships を返す。`date=YYYY-MM-DD` 指定時はその日の friend status を付与する。

### `POST /v1/friends`

Body:

```json
{"friend_id":"<uuid>"}
```

互いに block 関係がない場合、指定 user と friendship を作成する。

### `DELETE /v1/friends/{user_id}`

指定 user との friendship を解除する。Backend は caller が参加者であることを検証した上で、trusted server 側から対象 pair のみ削除する。

### `GET /v1/friend-requests/status?friend_id={user_id}`

Response:

```json
{"already_friend":false,"request_state":"outgoing","request_id":"<uuid>"}
```

`request_state` は `none` / `self` / `outgoing` / `incoming`。pending request がある場合は `request_id` を返す。

### `GET /v1/friend-requests?direction=all|incoming|outgoing`

設定画面の申請管理用。pending friend request を新しい順で返す。`direction` 省略時は `all`。

各 row は raw fields に加えて以下を持ちます。

- `from_user`
- `to_user`

Mobile は `X-Nomo-User-ID` と `from_user_id` / `to_user_id` を比較し、送信中 / 受信中に分けて表示する。

### `POST /v1/friend-requests`

Body:

```json
{"to_user_id":"<uuid>"}
```

申請を作成する。

### `PATCH /v1/friend-requests/{id}`

Body:

```json
{"status":"cancelled"}
```

Allowed status:

- `accepted`: recipient のみ
- `rejected`: recipient のみ
- `cancelled`: sender のみ

## User Safety

### `POST /v1/feed-hidden-drink-logs`

Body:

```json
{"drink_log_id":"<uuid>"}
```

自分の feed から対象投稿を非表示にする。通報とは分ける。

### `DELETE /v1/feed-hidden-drink-logs/{drink_log_id}`

非表示を解除する。

### `POST /v1/user-mutes`

Body:

```json
{"target_user_id":"<uuid>"}
```

対象 user の投稿を自分の feed から除外する。

### `GET /v1/user-mutes`

自分が mute している user profile 一覧を返す。設定画面の解除 UI 用。

### `DELETE /v1/user-mutes/{user_id}`

mute を解除する。

### `POST /v1/user-blocks`

Body:

```json
{"target_user_id":"<uuid>"}
```

対象 user を block する。Backend は以下を行う。

- feed から対象 user の投稿を除外
- friend request / drink invite 作成時に block 関係を拒否
- block 作成時に既存 friendship / pending friend request / pending drink invite を整理

### `GET /v1/user-blocks`

自分が block している user profile 一覧を返す。設定画面の解除 UI 用。

### `DELETE /v1/user-blocks/{user_id}`

block を解除する。解除しても過去に整理した friendship / request / invite は自動復元しない。

## Media

### `POST /v1/media/upload-url`

Drink log photo 用の Supabase Storage signed upload URL を返す。

### `POST /v1/media/display-url`

Body:

```json
{"path":"users/<user_id>/drink_logs/photo.jpg"}
```

Storage display 用 signed URL を Backend から返す。Mobile は raw `photo_path` を受け取ったらこの endpoint を優先する。

Drink log 削除時は Backend が `nomo-photos` object cleanup を best-effort で行う。

### `GET /v1/admin/media/orphan-drink-log-photos?user_id=<uuid>&limit=100`

指定 user の `users/<user_id>/drink_logs` prefix を Storage から確認し、`drink_logs.photo_path` に存在しない object path を候補として返す。実削除はしない。

## Notifications / Outbox

### `PUT /v1/me/push-token`

Current device token を登録 / refresh する。

```json
{"token":"<fcm-token>","platform":"ios"}
```

### `DELETE /v1/me/push-token`

Logout / token refresh 時に current or previous device token を best-effort で削除する。

```json
{"token":"<fcm-token>"}
```

FCM が token-specific invalid response を返した場合、Backend は service role で `push_tokens` から対象 token を削除する。

Domain events:

- `drink_invite.created`
- `drink_invite.accepted`
- `friend_request.created`
- `friend_request.accepted`
- `drink_log.tagged`
- `drink_log.liked`
- `drink_log.reported`
- `system_notification.created`

Backend は event を `notification_outbox` に保存し、in-process dispatch 後に `processed` / `failed` を更新します。

### `GET /v1/admin/notification-outbox?status=pending|failed|processed|all`

Outbox の直近 rows を確認する。

### `POST /v1/admin/notification-outbox/process?limit=50`

Due な `pending` / `failed` outbox rows を再処理する。
本番 Render cron は課金対象のため、現時点では作成しない。ユーザー数が増えて Pro / paid cron を使う必要が出るまでは、必要時にこの admin endpoint を手動実行する。

将来 cron を有効化する場合は `/nomo-notification-worker` を 5分ごとに実行し、production の `SUPABASE_SERVICE_ROLE_KEY` と `FCM_SERVICE_ACCOUNT_JSON` を設定する。

## Moderation

### `POST /v1/drink-logs/{id}/report`

Body:

```json
{"reason":"harassment"}
```

Allowed reason:

- `spam`
- `harassment`
- `inappropriate`
- `violence`
- `minor_safety`
- `other`

同じ user が同じ drink log を再 report した場合は duplicate として扱う。

### `GET /v1/admin/drink-log-reports?status=pending|reviewing|resolved|dismissed|all`

通報 queue を取得する。

### `PATCH /v1/admin/drink-log-reports/{id}`

Body:

```json
{"status":"resolved","moderation_note":"checked"}
```

Allowed status:

- `pending`
- `reviewing`
- `resolved`
- `dismissed`
