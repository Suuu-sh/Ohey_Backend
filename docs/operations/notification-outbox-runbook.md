# Notification outbox manual runbook

Last updated: 2026-06-09

Render cron is intentionally disabled for production because Render Cron Jobs are paid. Until user volume justifies Pro / paid cron, notification retry is operated manually.

## When to use

Use this runbook when one of these happens:

- push delivery or in-app notification creation failed temporarily
- `notification_outbox` has `pending` / `failed` rows
- an admin sees reports of missing push notifications
- after Firebase / database incident recovery

Normal request handling still creates and dispatches notifications in-process. This runbook only covers retrying rows that were stored in `notification_outbox`.

## Required access

- Clerk admin user email included in `OHEY_ADMIN_EMAILS`
- Backend production URL
- Valid Clerk session token from the admin app/session

Never expose `CLERK_SECRET_KEY`, `DATABASE_URL`, or Firebase service-account values to Mobile clients.

## Inspect outbox

```bash
curl -sS \
  -H "Authorization: Bearer $ADMIN_ACCESS_TOKEN" \
  -H "X-Ohey-User-ID: $ADMIN_USER_ID" \
  "https://ohey-backend.onrender.com/v1/admin/notification-outbox?status=failed" | jq .
```

Supported `status` values:

- `pending`
- `failed`
- `processed`
- `all`

Check:

- `event_kind`
- `aggregate_type` / `aggregate_id`
- `recipient_user_id`
- `attempts`
- `last_error`
- `next_attempt_at`

## Retry manually

```bash
curl -sS -X POST \
  -H "Authorization: Bearer $ADMIN_ACCESS_TOKEN" \
  -H "X-Ohey-User-ID: $ADMIN_USER_ID" \
  "https://ohey-backend.onrender.com/v1/admin/notification-outbox/process?limit=50" | jq .
```

Expected response:

```json
{
  "processed_count": 0,
  "failed_count": 0,
  "skipped_count": 0
}
```

Run a second `GET` after retry and confirm failed/pending rows decreased or `last_error` changed.

## Admin UI readiness

The current API is already shaped for a future admin UI button:

- list failed rows: `GET /v1/admin/notification-outbox?status=failed`
- retry: `POST /v1/admin/notification-outbox/process?limit=50`
- show result counts from the response

When adding the UI, keep retry as an explicit admin action rather than automatic background polling.

## When to enable cron later

Enable Render cron only after user volume or notification reliability requirements justify paid cron/Pro.

The previous standalone notification worker binary was removed while cron is unused. If cron is needed later, add a new worker entry point and Render service deliberately at that time.
