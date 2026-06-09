# Auth / seed / admin account policy

Last updated: 2026-06-09

## Decisions

- Clerk is the source of identity.
- `profiles.id` stores the Clerk user id.
- Profile bootstrap is owned by the backend and is idempotent.
- Dev seed SQL creates application data only. It does not create identity users.
- Admin is not a DB role and not a seeded special table row. Backend admin endpoints check the authenticated Clerk user email against `OHEY_ADMIN_EMAILS` in Render env.

## Why

メリット:

- Auth provider and application database ownership are separated.
- Mobile が初回起動時に profile 作成 race を起こしても backend 側で安全に処理できる。
- admin 権限を DB seed に混ぜないので、dev/prod の seed 誤適用リスクが下がる。
- `CLERK_SECRET_KEY` は Backend/Actions/ops script だけが使い、Mobile に露出しない。

デメリット:

- production smoke 用の admin/test user は Clerk 上に明示的に作る必要がある。
- `OHEY_ADMIN_EMAILS` の Render env 設定漏れがあると admin UI が使えない。
- Clerk issuer / JWKS / secret の変更は signup/profile bootstrap に直結するため、runtime smoke が必須。

## Dev operation

1. Apply the Postgres baseline migration to the dev Neon database.
2. Create required test/admin users in Clerk.
3. Set `OHEY_ADMIN_EMAILS` for the dev backend environment.
4. Confirm Backend through `scripts/ohey_backend_smoke.py` with a Clerk session token.

## Production operation during pre-release

- No rich seed in production.
- TestFlight tester/admin accounts are normal Clerk users.
- If production data is reset by baseline, recreate only the minimum test/admin accounts needed for TestFlight.
- Keep production credentials in `/Users/yota/Projects/Secrets/Ohey`, Render secrets, and Clerk secrets only.
