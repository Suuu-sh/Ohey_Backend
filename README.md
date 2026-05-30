# Ohey Backend

Go API for Ohey. It proxies authenticated requests to Supabase/PostgREST using the caller's Supabase JWT so RLS remains enforced by Supabase.

## Architecture

Ohey Backend uses a lightweight, AI-friendly architecture policy for future
feature work:

- [AI駆動開発向け Backend 設計ガイド](docs/architecture/ai-driven-feature-slice.md)
- [ADR 0001: AI駆動開発向けに Feature Slice 型の軽量 Clean Architecture を採用する](docs/adr/0001-ai-driven-feature-slice-clean-architecture.md)
- [Backend API Contract](docs/api/backend-api-contract.md)

## Local run

```sh
cp .env.example .env
# set SUPABASE_ANON_KEY from /Users/yota/Projects/Secrets/Nomo/supabase_dev-ohey.md
export $(grep -v '^#' .env | xargs)
go run ./cmd/api
```

Health check:

```sh
curl http://localhost:8080/healthz
```

Authenticated requests must include:

- `Authorization: Bearer <supabase access token>`
- `X-Ohey-User-ID: <auth.users.id>`

## Endpoints

- `GET /healthz`
- `GET /v1/me/profile`
- `PUT /v1/me/profile`
- `PATCH /v1/me/profile`
- `GET /v1/profiles/by-user-id/{user_id}`
- `GET /v1/friends`
- `POST /v1/friends`
- `DELETE /v1/friends/{id}`
- `PUT /v1/friends/{id}/favorite`
- `GET /v1/friend-requests?direction=all|incoming|outgoing`
- `GET /v1/friend-requests/status?friend_id={id}`
- `POST /v1/friend-requests`
- `PATCH /v1/friend-requests/{id}`
- `GET /v1/memories`
- `POST /v1/memories`
- `DELETE /v1/memories/{id}`
- `PUT /v1/memories/{id}/like`
- `DELETE /v1/memories/{id}/like`
- `POST /v1/memories/{id}/report`
- `POST /v1/memory-hides`
- `DELETE /v1/memory-hides/{memory_id}`
- `POST /v1/user-mutes`
- `GET /v1/user-mutes`
- `DELETE /v1/user-mutes/{user_id}`
- `POST /v1/user-blocks`
- `GET /v1/user-blocks`
- `DELETE /v1/user-blocks/{user_id}`
- `POST /v1/media/upload-url`
- `POST /v1/media/display-url`
- `GET /v1/daily-status?date=YYYY-MM-DD`
- `GET /v1/daily-statuses/month?month=YYYY-MM`
- `PUT /v1/daily-status`
- `GET /v1/invites/today-reservations?date=YYYY-MM-DD`
- `GET /v1/invites/incoming-pending?date=YYYY-MM-DD`
- `GET /v1/invites/outgoing-active?date=YYYY-MM-DD`
- `POST /v1/invites`
- `PATCH /v1/invites/{id}`
- `GET /v1/notifications`
- `PATCH /v1/notifications/read-all`
- `PUT /v1/me/push-token`
- `DELETE /v1/me/push-token`
- `DELETE /v1/me/account`

## Admin endpoints

Admin operations run only through the trusted backend so the Supabase service
role key is never shipped to Flutter. Set these backend environment variables in
dev and production before using `/v1/admin/*`:

- `SUPABASE_SERVICE_ROLE_KEY`

Admin access is intentionally hard-limited to the Supabase Auth user whose
email is `yisshiki39@gmail.com`.

Available endpoints:

- `GET /v1/admin/me`
- `GET /v1/admin/users`
- `POST /v1/admin/users`
- `PATCH /v1/admin/users/{id}`
- `DELETE /v1/admin/users/{id}`
- `GET /v1/admin/memories`
- `GET /v1/admin/memory-reports`
- `PATCH /v1/admin/memory-reports/{id}`
- `GET /v1/admin/notification-outbox`
- `POST /v1/admin/notification-outbox/process`
- `GET /v1/admin/media/orphan-memory-photos`
- `POST /v1/admin/memories`
- `PATCH /v1/admin/memories/{id}`
- `DELETE /v1/admin/memories/{id}`


## Push notifications

Device push delivery uses Firebase Cloud Messaging for APNs/TestFlight builds.

Required setup:

1. Add `GoogleService-Info.plist` to the iOS Runner target.
2. Add `google-services.json` to `android/app/`.
3. Enable Push Notifications / APNs for the app identifier in Apple Developer and upload/configure the APNs key or certificate in Firebase.
4. Set `FCM_SERVICE_ACCOUNT_JSON` on the backend to the Firebase service account JSON (raw JSON or base64-encoded JSON).
5. Run the Supabase migration that creates `public.push_tokens`.

When the app starts on iOS or Android, it asks notification permission and registers the FCM token through `PUT /v1/me/push-token`. The backend sends pushes when it creates notifications for likes, friend requests, and invites.
