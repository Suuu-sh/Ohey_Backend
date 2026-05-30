# Auth / seed / admin account policy

Last updated: 2026-05-28

## Decisions

- Supabase Auth is the source of identity.
- `profiles.id` is always the same UUID as `auth.users.id`.
- A private Auth trigger `private.handle_new_user()` bootstraps `profiles` exactly once for new users.
- Dev seed SQL creates normal confirmed Auth users only. It does not create tables/policies/grants.
- Admin is not a DB role and not a seeded special table row. Backend admin endpoints check the authenticated Supabase user email against `OHEY_ADMIN_EMAILS` in Render env.

## Why

メリット:

- user id / profile id の二重管理を避けられる。
- Mobile が初回起動時に profile 作成 race を起こしても `on conflict` で安全。
- admin 権限を DB seed に混ぜないので、dev/prod の seed 誤適用リスクが下がる。
- service role key は Backend/Actions/ops script だけが使い、Mobile に露出しない。

デメリット:

- production smoke 用の admin/test user は Auth 上に明示的に作る必要がある。
- `OHEY_ADMIN_EMAILS` の Render env 設定漏れがあると admin UI が使えない。
- Auth trigger の変更は signup/profile bootstrap に直結するため、migration dry-run と runtime smoke が必須。

## Dev operation

1. Apply Supabase baseline migration to `dev-ohey`.
2. Run one of the dev seed SQL files after setting `app.seed_password`.
3. Confirm sign-in through `scripts/ohey_supabase_runtime_check.py`.
4. Confirm Backend through `scripts/ohey_backend_smoke.py`.

## Production operation during pre-release

- No rich seed in production.
- TestFlight tester/admin accounts are normal Supabase Auth users.
- If production data is reset by baseline, recreate only the minimum test/admin accounts needed for TestFlight.
- Keep production credentials in `/Users/yota/Projects/Secrets/Nomo` and Render/Supabase secrets only.
