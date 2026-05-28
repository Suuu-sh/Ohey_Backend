# Nomo production release runbook

Last updated: 2026-05-28

This runbook defines when to move `development` into `main` and what must be checked together for Backend, Supabase, and TestFlight. Nomo is pre-release, so normal implementation work still lands on `development`; production reflection happens as one coordinated release window.

## Release gate

Do not merge `development` to `main` until all of the following are true.

- GitHub Actions `Supabase Dev Migrate` on `development` is green for the latest migration-changing push.
- Backend tests pass: `go test ./...`.
- Mobile checks pass: `flutter analyze` and `flutter test`.
- Supabase/RLS static verifier passes from Mobile: `python3 scripts/verify_supabase_rls_contract.py`.
- Pre-release smoke checklist has been run against dev Render/Supabase on iOS Simulator or a dev device.
- Known manual-only checks are listed with owner/date before release.
- Render paid cron remains disabled unless we explicitly decide to pay for cron. Notification outbox retry stays manual until then.

## Release sequence

1. Confirm Backend and Mobile are both on a clean, up-to-date `development` branch.
2. Review migration files and operational docs for destructive changes. Treat Supabase migrations as forward-only.
3. Merge/sync Backend `development` into Backend `main`.
4. Confirm production Render backend deploy from `main` succeeds and `/healthz` responds.
5. Merge/sync Mobile `development` into Mobile `main`.
6. Confirm the production Supabase migration workflow for `main` succeeds.
7. Build/upload TestFlight if the Mobile app changed.
8. Run post-release smoke against production/TestFlight:
   - login/profile bootstrap
   - drink log create/delete
   - photo upload/display
   - feed pagination
   - invite/accept
   - block/mute/hide/report
   - push token register/logout cleanup
   - admin moderation list/status/note/photo preview/post delete
9. Record the release date, Backend commit, Mobile commit, migration workflow URLs, and any skipped checks.

## Rollback / recovery

- Backend: use Render rollback to the previous successful production deploy if the app is unhealthy.
- Mobile: do not promote the TestFlight build; upload a fixed build if needed.
- Supabase: do not rely on down migrations. For destructive changes, prepare a forward recovery migration before release.
- Notifications: if outbox retry fails while cron is disabled, use the manual admin endpoint runbook: `notification-outbox-runbook.md`.
- Media cleanup: do not hard-delete objects automatically during release. Use orphan detection first, then delete only after admin review/retention decision.

## Deferred production decisions

- Admin-level user hide is not implemented yet because the current user delete endpoint is destructive and user block/mute is per-viewer. Add a non-destructive backend moderation state first, for example `profiles.hidden_at` / `profiles.moderation_status`, then hide users from feed/invite/search/admin surfaces consistently.
- Render cron can be enabled later when user volume justifies paid cron. Until then, manual notification outbox retry is the supported operation.
