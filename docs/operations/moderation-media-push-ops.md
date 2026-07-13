# Moderation / media / push operations

Last updated: 2026-07-14

## Moderation duty

Ohey contains friend-visible user-generated content. Keep at least one verified
Clerk moderator email in production `OHEY_ADMIN_EMAILS` and publish a working
support contact in the app, terms, privacy policy, and App Store metadata.

Target response times:

- imminent danger, minor safety, credible violence: acknowledge immediately and act within 24 hours;
- harassment, sexual/inappropriate content, impersonation: review within 48 hours;
- spam and other reports: review within 72 hours.

Escalate emergencies to appropriate local authorities; preserve only the minimum
evidence necessary and restrict access.

## Inspect pending user reports

The current admin API does not expose user reports. Use a read-only/direct Neon
administration session and never the pooled runtime URL:

```sql
select
  ur.id,
  ur.reason,
  ur.status,
  ur.created_at,
  reporter.user_id as reporter_ohey_id,
  reported.user_id as reported_ohey_id,
  reported.display_name as reported_display_name
from user_reports ur
join profiles reporter on reporter.id = ur.reporter_user_id
join profiles reported on reported.id = ur.reported_user_id
where ur.status in ('pending', 'reviewing')
order by
  case ur.reason
    when 'minor_safety' then 0
    when 'violence' then 1
    when 'harassment' then 2
    else 3
  end,
  ur.created_at asc;
```

Start review atomically:

```sql
update user_reports
set status = 'reviewing', reviewed_by_user_id = :moderator_profile_id,
    reviewed_at = now(), updated_at = now()
where id = :report_id and status = 'pending';
```

Document facts, not speculation, in `moderation_note`. Use `resolved` when action
was required and completed, or `dismissed` when the report was not substantiated.
Do not paste user content into external tickets unless access and retention are
appropriate.

Available safety controls:

- user block: removes friendship and closes pending friend requests/invites;
- user mute: hides content for the muting user;
- admin yurubo delete: removes violating content;
- Clerk user restriction/deletion: use for serious or repeated account abuse;
- support follow-up: notify the reporter only with information safe to disclose.

After action, verify the content/account is no longer available to an ordinary user
and record reviewer, timestamps, reason, action, and follow-up.

## Media

Media upload routes are currently not enabled in the router. Do not configure or
advertise R2 upload capability until upload type/size validation, malware/content
review, signed URL expiry, deletion, and abuse operations are implemented and
smoke-tested. Existing avatar values are structured in-app character encodings,
not arbitrary user photo uploads.

## Push and notification outbox

- FCM service-account JSON belongs only in Render/provider secret storage.
- Treat push tokens as account-linked identifiers; never print them in logs or tickets.
- Inspect and retry failed events with `notification-outbox-runbook.md`.
- A repeated invalid/unregistered FCM token should be removed from `push_tokens`.
- For a Firebase credential incident, rotate the service account, update Render,
  redeploy, and check outbox failures without exposing the old JSON.

## Weekly operational check

- [ ] Pending/reviewing reports and oldest age
- [ ] Working support mailbox and moderator access
- [ ] Failed/pending notification outbox rows
- [ ] Clerk/Render/Neon/Firebase security alerts
- [ ] Production legal endpoints and account deletion
- [ ] Secrets approaching expiration and least-privilege access
