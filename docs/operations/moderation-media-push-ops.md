# Moderation, media cleanup, and push token operations

Last updated: 2026-05-28

## Admin moderation flow

Report reasons:

- `spam`
- `harassment`
- `inappropriate`
- `violence`
- `minor_safety`
- `other`

Statuses:

- `pending`: report needs review
- `reviewing`: admin has started review
- `resolved`: action completed / report accepted
- `dismissed`: no action needed / duplicate / invalid

Operational rules:

1. Treat duplicate reports from the same reporter/log as idempotent.
2. Keep the report hidden for the reporter immediately.
3. Admin reviews the report queue from the admin UI.
4. Use `resolved` only after action is taken or the content is confirmed unsafe.
5. Use `dismissed` for duplicate/no-issue reports.
6. Keep `moderation_note` short and non-sensitive.

## Media cleanup policy

Current state:

- Drink log deletion performs best-effort Storage object cleanup.
- `GET /v1/admin/media/orphan-drink-log-photos?user_id=<uuid>&limit=100` lists orphan candidates only.
- No automatic orphan deletion is enabled.

Policy until launch:

- Do not auto-delete orphan candidates.
- Admin confirms candidates first.
- Retain orphan media at least 7 days before manual deletion.
- If a report hides/removes content, confirm visibility before deleting Storage objects.

Future endpoint candidate:

- `DELETE /v1/admin/media/orphan-drink-log-photos`
- require explicit `paths` list
- return deleted / skipped / failed counts

## Push token lifecycle

Current behavior:

- Login/startup registers current FCM token via `PUT /v1/me/push-token`.
- Firebase token refresh registers the new token and best-effort unregisters the previous token.
- Logout best-effort unregisters the current token via `DELETE /v1/me/push-token`.
- FCM invalid-token responses delete the token from `push_tokens` using service role.

Do not keep sending to tokens that returned `UNREGISTERED`, `SENDER_ID_MISMATCH`, or token-specific `INVALID_ARGUMENT`.

Non-token FCM failures, such as auth/config errors, should remain visible in logs/outbox and must not delete user tokens automatically.
