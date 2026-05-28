# Nomo pre-release smoke test checklist

Last updated: 2026-05-28

Run this before TestFlight / production release. For Nomo dev checks, use iOS Simulator connected to dev Render/Supabase unless explicitly testing production/TestFlight.

## Account / profile

- [ ] Login succeeds with a normal user.
- [ ] Profile bootstrap creates the first profile exactly once.
- [ ] Profile update changes allowed fields only.
- [ ] Gender cannot be changed after initial profile creation.
- [ ] Admin account can open admin screen; normal account cannot.

## Memories / media

- [ ] Create memory without photo.
- [ ] Create memory with photo upload URL.
- [ ] Created photo displays via Backend display URL.
- [ ] Tagged friends are saved and shown.
- [ ] Delete memory removes DB row and best-effort photo cleanup does not error.
- [ ] Home feed still loads after deletion.

## Home feed

- [ ] Initial feed loads.
- [ ] Feed pagination loads additional rows near the end.
- [ ] Official posts, friend posts, and own posts appear in expected order.
- [ ] Hidden/reported/blocked/muted users are excluded before ranking.

## Invites / friends

- [ ] Friend request create succeeds.
- [ ] Friend request accept creates friendship.
- [ ] Invite create succeeds for an available friend.
- [ ] Invite accept updates both users' relevant views.
- [ ] Blocked users cannot create friend requests or invites.

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
- [ ] Invite created / accepted notifications appear.
- [ ] Memory like / tag notifications appear.
- [ ] Logout unregisters the current push token best-effort.
- [ ] Invalid FCM token failure deletes that token from `push_tokens`.

## Outbox manual retry

- [ ] `GET /v1/admin/notification-outbox?status=failed` works for admin.
- [ ] `POST /v1/admin/notification-outbox/process?limit=50` works for admin.
- [ ] Render cron is not created unless paid cron is intentionally enabled.

## Execution log

### 2026-05-28 dev Backend/API smoke after memories/invites rename

- Backend/Supabase target: `https://dev-nomo-backend.onrender.com` + dev-nomo Supabase.
- Render deploy: `dev-nomo-backend` live on Backend commit `2fa7ff2` (`Refactor app domain to memories and invites`).
- GitHub Actions: latest `Supabase Dev Migrate` run on `development` succeeded: https://github.com/Suuu-sh/Nomo_Mobile/actions/runs/26572788398
- Executed with prepared dev users: login, profile bootstrap, daily status/month endpoint, friendship, signed photo upload URL, actual signed Storage upload, memory create, memory list, home feed with uploaded photo, like, report, hide/unhide, invite create/list/accept/reservations, mute/unmute, block/unblock, user report, notifications list.
- Verified old REST tables return missing: `drink_logs`, `drink_invites`, `drink_log_reports`, `feed_hidden_drink_logs`.
- Not covered by API smoke: real APNs/FCM device delivery and manual visual QA on the latest Simulator build.

### 2026-05-28 dev Simulator partial

- Device: iOS Simulator `iPhone 17`.
- Backend/Supabase target: dev environment.
- Result: build/run succeeded and an existing Admin session loaded. Feed/profile/settings surfaces rendered. Admin screen and 通報 tab loaded; status/note/post-delete controls were visible without executing destructive actions.
- GitHub Actions: latest `Supabase Dev Migrate` run on `development` succeeded: https://github.com/Suuu-sh/Nomo_Mobile/actions/runs/26548996618
- Not fully executed in this pass: mutating manual flows (`memory create`, `photo upload`, `invite/accept`, `block/mute/hide/report`) and push notification delivery. Run those with prepared dev accounts before production/TestFlight release.

