# Neon migration runbook

Last updated: 2026-06-09

Ohey backend uses Clerk for auth and Neon/Postgres for application data.

## Branches

- production: Render `ohey-backend`
- development: Render `dev-ohey-backend`

Use pooled Neon connection strings (`*-pooler`) for Render runtime. Use direct Neon connection strings for schema migration, dump, restore, and admin tooling.

## Baseline schema

The backend-owned schema is stored in:

```bash
Backend/db/neon_baseline.sql
```


Notes:

- `notifications.memory_id` is kept as a nullable compatibility column for historical migrated rows. Current backend code does not use the legacy memories/storage feature.
- Existing migrated `profiles.clerk_user_id` values may be null until each legacy user is mapped to a Clerk user or recreated through Clerk.

Apply to an empty Neon database with:

```bash
psql "$DIRECT_DATABASE_URL" -v ON_ERROR_STOP=1 -f Backend/db/neon_baseline.sql
```

## Data migration verification

After restoring data, compare counts for the backend-owned tables:

```sql
select 'profiles', count(*) from profiles union all
select 'friendships', count(*) from friendships union all
select 'friend_requests', count(*) from friend_requests union all
select 'daily_statuses', count(*) from daily_statuses union all
select 'friend_groups', count(*) from friend_groups union all
select 'friend_group_members', count(*) from friend_group_members union all
select 'user_blocks', count(*) from user_blocks union all
select 'user_mutes', count(*) from user_mutes union all
select 'user_reports', count(*) from user_reports union all
select 'wish_items', count(*) from wish_items union all
select 'yurubos', count(*) from yurubos union all
select 'yurubo_reactions', count(*) from yurubo_reactions union all
select 'hidden_yurubos', count(*) from hidden_yurubos union all
select 'yurubo_visibility_groups', count(*) from yurubo_visibility_groups union all
select 'invites', count(*) from invites union all
select 'notifications', count(*) from notifications union all
select 'push_tokens', count(*) from push_tokens union all
select 'notification_outbox', count(*) from notification_outbox
order by 1;
```

## 2026-06-09 migration result

Initial production data migration completed with matching row counts between source, Neon production, and Neon development branches.

Render env was updated:

- `DATA_STORE=neon`
- `AUTH_PROVIDER=clerk`
- `DATABASE_URL=<Neon pooled URL>`
- `CLERK_ISSUER`
- `CLERK_JWKS_URL`
- `CLERK_SECRET_KEY`
