# Nomo Backend

Go API for Nomo. It proxies authenticated requests to Supabase/PostgREST using the caller's Supabase JWT so RLS remains enforced by Supabase.

## Local run

```sh
cp .env.example .env
# set SUPABASE_ANON_KEY from /Users/yota/Projects/Secrets/Nomo/supabase_dev-nomo.md
export $(grep -v '^#' .env | xargs)
go run ./cmd/api
```

Health check:

```sh
curl http://localhost:8080/healthz
```

Authenticated requests must include:

- `Authorization: Bearer <supabase access token>`
- `X-Nomo-User-ID: <auth.users.id>`

## Endpoints

- `GET /healthz`
- `GET /v1/me/profile`
- `PUT /v1/me/profile`
- `PATCH /v1/me/profile`
- `GET /v1/profiles/by-user-id/{user_id}`
- `GET /v1/friends`
- `POST /v1/friends`
- `PUT /v1/friends/{id}/favorite`
- `GET /v1/friend-requests/status?friend_id={id}`
- `POST /v1/friend-requests`
- `PATCH /v1/friend-requests/{id}`
- `GET /v1/drink-logs`
- `POST /v1/drink-logs`
- `DELETE /v1/drink-logs/{id}`
- `PUT /v1/drink-logs/{id}/like`
- `DELETE /v1/drink-logs/{id}/like`
- `POST /v1/drink-logs/{id}/report`
- `GET /v1/daily-status?date=YYYY-MM-DD`
- `PUT /v1/daily-status`
- `GET /v1/drink-invites/today-reservations?date=YYYY-MM-DD`
- `GET /v1/drink-invites/incoming-pending?date=YYYY-MM-DD`
- `GET /v1/drink-invites/outgoing-active?date=YYYY-MM-DD`
- `POST /v1/drink-invites`
- `PATCH /v1/drink-invites/{id}`
- `GET /v1/notifications`
- `PATCH /v1/notifications/read-all`
- `PUT /v1/me/push-token`

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
- `GET /v1/admin/drink-logs`
- `POST /v1/admin/drink-logs`
- `PATCH /v1/admin/drink-logs/{id}`
- `DELETE /v1/admin/drink-logs/{id}`


## Push notifications

Device push delivery uses Firebase Cloud Messaging for APNs/TestFlight builds.

Required setup:

1. Add `GoogleService-Info.plist` to the iOS Runner target.
2. Add `google-services.json` to `android/app/`.
3. Enable Push Notifications / APNs for the app identifier in Apple Developer and upload/configure the APNs key or certificate in Firebase.
4. Set `FCM_SERVICE_ACCOUNT_JSON` on the backend to the Firebase service account JSON (raw JSON or base64-encoded JSON).
5. Run the Supabase migration that creates `public.push_tokens`.

When the app starts on iOS or Android, it asks notification permission and registers the FCM token through `PUT /v1/me/push-token`. The backend sends pushes when it creates notifications for likes, friend requests, and drink invites.
