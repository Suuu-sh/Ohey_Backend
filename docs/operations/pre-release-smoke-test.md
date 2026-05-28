# Nomo pre-release smoke test checklist

Last updated: 2026-05-28

Run this before TestFlight / production release. For Nomo dev checks, use iOS Simulator connected to dev Render/Supabase unless explicitly testing production/TestFlight.

## Account / profile

- [ ] Login succeeds with a normal user.
- [ ] Profile bootstrap creates the first profile exactly once.
- [ ] Profile update changes allowed fields only.
- [ ] Gender cannot be changed after initial profile creation.
- [ ] Admin account can open admin screen; normal account cannot.

## Drink logs / media

- [ ] Create drink log without photo.
- [ ] Create drink log with photo upload URL.
- [ ] Created photo displays via Backend display URL.
- [ ] Tagged friends are saved and shown.
- [ ] Delete drink log removes DB row and best-effort photo cleanup does not error.
- [ ] Home feed still loads after deletion.

## Home feed

- [ ] Initial feed loads.
- [ ] Feed pagination loads additional rows near the end.
- [ ] Official posts, friend posts, and own posts appear in expected order.
- [ ] Hidden/reported/blocked/muted users are excluded before ranking.

## Invites / friends

- [ ] Friend request create succeeds.
- [ ] Friend request accept creates friendship.
- [ ] Drink invite create succeeds for an available friend.
- [ ] Drink invite accept updates both users' relevant views.
- [ ] Blocked users cannot create friend requests or drink invites.

## Safety / moderation

- [ ] Hide feed item removes it locally and after reload.
- [ ] Mute user removes their posts from feed.
- [ ] Block user removes relationship/pending requests/invites.
- [ ] Report creates or reuses a report row and hides the post for the reporter.
- [ ] Admin report list shows pending reports.
- [ ] Admin can mark report as reviewing / resolved / dismissed.

## Notifications / push

- [ ] Register push token after login.
- [ ] Friend request created notification appears.
- [ ] Friend request accepted notification appears.
- [ ] Drink invite created / accepted notifications appear.
- [ ] Drink log like / tag notifications appear.
- [ ] Logout unregisters the current push token best-effort.
- [ ] Invalid FCM token failure deletes that token from `push_tokens`.

## Outbox manual retry

- [ ] `GET /v1/admin/notification-outbox?status=failed` works for admin.
- [ ] `POST /v1/admin/notification-outbox/process?limit=50` works for admin.
- [ ] Render cron is not created unless paid cron is intentionally enabled.

## Execution log

### 2026-05-28 dev Simulator partial

- Device: iOS Simulator `iPhone 17`.
- Backend/Supabase target: dev environment.
- Result: build/run succeeded and an existing Admin session loaded. Feed/profile/settings surfaces rendered.
- GitHub Actions: latest `Supabase Dev Migrate` run on `development` succeeded: https://github.com/Suuu-sh/Nomo_Mobile/actions/runs/26548996618
- Not fully executed in this pass: mutating manual flows (`drink log create`, `photo upload`, `invite/accept`, `block/mute/hide/report`) and push notification delivery. Run those with prepared dev accounts before production/TestFlight release.

