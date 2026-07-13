# Pre-release smoke test

Last updated: 2026-07-14

Run first against dev (Clerk dev + Neon dev + dev Render), then against production
with dedicated disposable/test accounts. Do not use a local backend for Simulator
checks and never paste tokens into the document.

## Automated gate

- [ ] Backend `go test ./...`
- [ ] Backend `go vet ./...`
- [ ] Backend `govulncheck ./...`
- [ ] Mobile `flutter analyze --no-fatal-infos`
- [ ] Mobile `flutter test`
- [ ] iOS Release/no-codesign build
- [ ] `Info.plist` and `PrivacyInfo.xcprivacy` lint

## Public edge and security

- [ ] `GET /health` returns 200 through Render and public proxy.
- [ ] Protected direct production-origin request returns 403.
- [ ] Protected proxy request without Bearer token returns 401.
- [ ] API response includes `X-Request-ID`, HSTS, CSP, nosniff, and `Cache-Control: no-store` for `/v1/*`.
- [ ] An unapproved browser origin receives no CORS allow-origin response.
- [ ] Oversized/malformed JSON is rejected without a stack trace or internal error text.
- [ ] Logs contain request ID/method/path/status/duration and no credentials.

## Authentication and account lifecycle

- [ ] Email sign-up, verification/client trust, sign-in, reset, and sign-out work.
- [ ] Google and Apple sign-in use production identities in TestFlight.
- [ ] Dev token is rejected by production and production token is rejected by dev.
- [ ] Profile bootstrap cannot create/update another user's profile.
- [ ] Account deletion requires an authenticated session and deletes the Clerk user and associated profile data.
- [ ] A disposable deleted account cannot continue using its previous token.

## Core data and authorization

Using users A, B, and C:

- [ ] A/B friendship request, accept/reject, duplicate, and removal flows.
- [ ] Daily status write/read and friend-only monthly status authorization.
- [ ] Friend group visibility does not expose a group post to C.
- [ ] Invite create/accept/reject/cancel and duplicate protection.
- [ ] Yurubo create/edit/delete/participation and owner authorization.
- [ ] Wish item create/edit/delete and visibility rules.
- [ ] Block immediately removes relationship/invite exposure in both directions.
- [ ] Mute hides content without granting access or changing ownership.

## Safety and moderation

- [ ] A can report B from the friend profile.
- [ ] A can report the author directly from B's yurubo action menu.
- [ ] Self-report, invalid reason, and report-spam rate limit are rejected.
- [ ] A moderator can inspect pending reports using the moderation runbook.
- [ ] Moderator can identify the reported account and take a documented action.
- [ ] Published support contact, terms, and privacy policy open from the app.

## Notification, ads, and purchases

- [ ] Push token register/unregister works on a real TestFlight device.
- [ ] Friend/invite/yurubo notification and retry outbox behavior is correct.
- [ ] UMP consent is evaluated before ATT and before loading ads.
- [ ] Production build contains no Google sample ad IDs and logs no IDFA.
- [ ] Plus purchase and restore work with sandbox/TestFlight.
- [ ] Plus entitlement removes ads and remains correct after relaunch/sign-in change.

## Failure cases

- [ ] Cold Render start shows a retryable UI rather than losing submitted data.
- [ ] Network timeout and 401 refresh do not cause duplicate writes.
- [ ] Backend panic test returns a generic 500 and records a request ID.
- [ ] Database/API provider outage is visible in logs and has a rollback/escalation owner.

Record commit IDs, build number, tester accounts (identifiers only), result, and any
waiver in the release evidence template. A waiver must have an owner and deadline.
