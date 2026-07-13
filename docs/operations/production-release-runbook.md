# Production release runbook

Last updated: 2026-07-14

This is the release authority for Ohey Backend and Mobile. A release is not ready
until every blocking item below is complete and evidence is recorded.

## 1. Roles and separation

- **Release owner**: runs the checklist and records commit/build identifiers.
- **Reviewer**: checks production configuration, privacy disclosures, and smoke results.
- **Moderator on call**: can access the admin account and respond to reports.
- Use separate Clerk, Neon, Firebase, Render, and mobile values for dev and production.
- Keep reusable credentials only in `/Users/yota/Projects/Secrets/Ohey`, provider
  secret stores, or GitHub environments. Never paste a credential into an issue,
  commit, build log, screenshot, or chat.

For a solo release, one person may fill all roles, but still perform the steps in
order and record the evidence.

## 2. Release blockers

Stop the release when any of these is true:

- `development` is not clean and synchronized with `origin/development`.
- Backend `go test`, `go vet`, or `govulncheck` fails.
- Mobile `flutter analyze`, `flutter test`, iOS Release build, or privacy manifest
  validation fails.
- A production secret is missing, a dev/test Clerk key is used, Neon TLS is not
  required, or the Render origin accepts protected direct requests.
- Account deletion, report, block, or legal-document access is unavailable.
- App Store Connect privacy answers do not match the archived app privacy report.
- There is no moderator able to respond to a safety report.
- A destructive database baseline is being proposed for a database containing data.

## 3. Configuration inventory

Confirm names and presence without printing values.

### Backend production

Required core configuration:

- `APP_ENV=production`
- `DATA_STORE=neon`
- pooled `DATABASE_URL` with `sslmode=require` (or stronger)
- `CLERK_ISSUER`, `CLERK_JWKS_URL`, `CLERK_AUDIENCE`
- Clerk production `CLERK_SECRET_KEY`
- `ALLOWED_ORIGINS` containing only owned HTTPS origins
- at least one `OHEY_ADMIN_EMAILS` address
- random `ORIGIN_VERIFY_SECRET` of at least 32 bytes, matching the Cloudflare Worker

Required for enabled production capabilities:

- `CLERK_WEBHOOK_SECRET`, `RESEND_API_KEY`, `RESEND_FROM_EMAIL`
- `FCM_SERVICE_ACCOUNT_JSON`
- R2 variables only if media endpoints are enabled

A direct request to `https://ohey-backend.onrender.com/v1/me/profile` without
`X-Origin-Verify` must return `403`. The same request through
`https://api.oheyapp.com` must reach application auth and return `401` without a
Bearer token.

### Mobile production

- `OHEY_ENV=production`
- `OHEY_BACKEND_URL=https://api.oheyapp.com`
- Clerk publishable key starts with `pk_live_`
- `CLERK_JWT_TEMPLATE=ohey-mobile`
- production Google OAuth client IDs and callback scheme
- production Firebase values
- production AdMob app/unit IDs; never Google sample IDs
- RevenueCat public iOS SDK key and matching entitlement/offering/product IDs
- App Store Connect signing values in the protected `testflight` environment

Only public client identifiers may be compiled with `--dart-define`. Secret,
password, private-key, service-account, database URL, and server token variables
must fail the TestFlight workflow guard.

## 4. Database change gate

1. Review every SQL statement. `db/migrations/000001_neon_baseline.sql` is for an
   **empty database only** and drops tables; never run it against production data.
2. Create a point-in-time restore point / branch before a production schema change.
3. Apply a new forward-only numbered migration with the direct (non-pooler) URL.
4. Record migration version and row-count checks.
5. Deploy backend code only after the compatible schema is available.
6. Do not use an untested down migration. Roll forward or restore to a verified
   branch when recovery is required.

See `docs/operations/neon-migration-runbook.md`.

## 5. Local and CI release gates

Backend:

```bash
gofmt -w cmd internal docs/legal
go test ./...
go vet ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.6.0 ./...
docker build -t ohey-backend:release-check .
```

Mobile:

```bash
dart format --output=none --set-exit-if-changed lib test
flutter analyze --no-fatal-infos
flutter test
plutil -lint ios/Runner/Info.plist ios/Runner/PrivacyInfo.xcprivacy
flutter build ios --release --no-codesign \
  --dart-define-from-file=config/firebase/prod.json \
  --dart-define=OHEY_ENV=production \
  --dart-define=OHEY_BACKEND_URL=https://api.oheyapp.com
```

The local production file must come from the protected Ohey secrets directory and
must remain ignored by Git. CI/TestFlight remains the authoritative signed build.

## 6. Deployment order

1. Verify dev Render health and run `pre-release-smoke-test.md` on dev.
2. Apply any production forward migration and verify it.
3. Deploy Backend `development` to production Render.
4. Check proxy headers, authentication rejection, legal documents, and authenticated
   production smoke with the dedicated production test account.
5. Merge/promote the reviewed Mobile commit to the release trigger branch according
   to the repository workflow.
6. Run `TestFlight Deploy`, record build number, and keep the build internal/private.
7. Run the full TestFlight device smoke, including purchase restore, notification,
   account deletion with a disposable account, report, block, and legal links.
8. Generate Xcode's privacy report from the archived build. Reconcile it with
   `Mobile/docs/release/app_privacy.md` and App Store Connect.
9. Submit/release only after all checks pass.

## 7. Post-deploy observation

For at least 30 minutes after backend deployment and again after tester rollout:

- check Render deploy status, restarts, latency, and 5xx responses;
- check `/health` through both Render and the public proxy;
- verify request IDs appear on API responses and in structured logs;
- inspect Clerk sign-in/sign-up failures and Resend webhook delivery;
- inspect Neon connection count, errors, and storage growth;
- inspect Firebase delivery errors and `notification_outbox` failed rows;
- confirm reports can be queried and a moderator is reachable.

Do not log Bearer tokens, IDFA values, passwords, database URLs, or service-account
JSON while diagnosing an incident.

## 8. Rollback

Backend-only regression:

1. Stop further mobile distribution.
2. Roll Render back to the last known-good image/commit.
3. Keep a backward-compatible schema. Do not run destructive SQL to match old code.
4. Re-run health and the affected smoke cases.

Mobile regression:

1. Remove the TestFlight group assignment or expire the affected build.
2. Keep the previous known-good build available to internal testers.
3. Fix on `development`, repeat all gates, and upload a new build number.

Credential exposure:

1. Revoke/rotate the credential at the provider first.
2. Update the protected secret store and dependent environments.
3. Redeploy affected services and invalidate sessions when applicable.
4. Review logs and provider audit events; document scope and follow-up.

## 9. Release evidence template

```text
Release date/time (JST):
Release owner / reviewer / moderator:
Backend commit and Render deploy ID:
Mobile commit and TestFlight build:
Database migration / restore point:
CI URLs:
Dev smoke result:
Production smoke result:
Privacy report reviewed:
Open risks and owner:
Rollback target:
```
