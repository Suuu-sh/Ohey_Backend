# Notification outbox manual runbook

Last updated: 2026-05-28

Render cron is intentionally disabled for production because Render Cron Jobs are paid. Until user volume justifies Pro / paid cron, notification retry is operated manually.

## When to use

Use this runbook when one of these happens:

- push delivery or in-app notification creation failed temporarily
- `notification_outbox` has `pending` / `failed` rows
- an admin sees reports of missing push notifications
- after Firebase / Supabase incident recovery

Normal request handling still creates and dispatches notifications in-process. This runbook only covers retrying rows that were stored in `notification_outbox`.

## Required access

- Admin Supabase Auth user included in `NOMO_ADMIN_EMAILS`
- Backend production URL
- Valid access token from the admin app/session

Never expose `SUPABASE_SERVICE_ROLE_KEY` or Firebase service-account values to Mobile clients.

## Inspect outbox

```bash
curl -sS \
  -H "Authorization: Bearer $ADMIN_ACCESS_TOKEN" \
  -H "X-Nomo-User-ID: $ADMIN_USER_ID" \
  "https://nomo-backend-nezf.onrender.com/v1/admin/notification-outbox?status=failed" | jq .
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
  -H "X-Nomo-User-ID: $ADMIN_USER_ID" \
  "https://nomo-backend-nezf.onrender.com/v1/admin/notification-outbox/process?limit=50" | jq .
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

Future cron settings:

- service name: `nomo-notification-outbox-worker`
- schedule: `*/5 * * * *`
- docker command: `/nomo-notification-worker`
- env: `NOMO_ENV`, `SUPABASE_URL`, `SUPABASE_ANON_KEY`, `SUPABASE_SERVICE_ROLE_KEY`, `FCM_SERVICE_ACCOUNT_JSON`, `ALLOWED_ORIGINS`
