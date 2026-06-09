begin;

create extension if not exists pgcrypto;
create schema if not exists private;

drop table if exists public.notification_outbox cascade;
drop table if exists public.notifications cascade;
drop table if exists public.invites cascade;
drop table if exists public.hidden_yurubos cascade;
drop table if exists public.yurubo_visibility_groups cascade;
drop table if exists public.yurubo_reactions cascade;
drop table if exists public.yurubos cascade;
drop table if exists public.wish_items cascade;
drop table if exists public.user_reports cascade;
drop table if exists public.user_mutes cascade;
drop table if exists public.user_blocks cascade;
drop table if exists public.friend_group_members cascade;
drop table if exists public.friend_groups cascade;
drop table if exists public.push_tokens cascade;
drop table if exists public.daily_statuses cascade;
drop table if exists public.friend_requests cascade;
drop table if exists public.friendships cascade;
drop table if exists public.profiles cascade;
drop table if exists public.app_schema_migrations cascade;

drop function if exists private.set_updated_at() cascade;
drop function if exists private.handle_friend_request_accepted() cascade;
drop function if exists private.profile_is_plus_unchanged(uuid, boolean) cascade;

create or replace function private.set_updated_at()
returns trigger
language plpgsql
set search_path = public
as $$
begin
  new.updated_at = now();
  return new;
end;
$$;

create table public.profiles (
  id uuid primary key default gen_random_uuid(),
  clerk_user_id text,
  user_id text not null unique,
  display_name text not null,
  character_key text not null default 'icon_smile',
  avatar_url text,
  is_plus boolean not null default false,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint profiles_user_id_format check (user_id ~ '^[a-zA-Z0-9_]{3,24}$'),
  constraint profiles_display_name_length check (char_length(display_name) between 1 and 40)
);
create unique index profiles_clerk_user_id_key on public.profiles (clerk_user_id) where clerk_user_id is not null;
create trigger profiles_set_updated_at before update on public.profiles for each row execute function private.set_updated_at();

create or replace function private.profile_is_plus_unchanged(profile_id uuid, requested_is_plus boolean)
returns boolean language sql stable security definer set search_path = public as $$
  select coalesce((select p.is_plus = requested_is_plus from public.profiles p where p.id = profile_id), false);
$$;

create table public.friendships (
  id uuid primary key default gen_random_uuid(),
  user_a_id uuid not null references public.profiles(id) on delete cascade,
  user_b_id uuid not null references public.profiles(id) on delete cascade,
  created_at timestamptz not null default now(),
  is_favorite boolean not null default false,
  constraint friendships_no_self check (user_a_id <> user_b_id),
  constraint friendships_ordered check (user_a_id < user_b_id),
  unique (user_a_id, user_b_id)
);
create index idx_friendships_user_b_id on public.friendships(user_b_id);
create index idx_friendships_user_a_favorite on public.friendships(user_a_id, is_favorite);
create index idx_friendships_user_b_favorite on public.friendships(user_b_id, is_favorite);

create table public.friend_requests (
  id uuid primary key default gen_random_uuid(),
  from_user_id uuid not null references public.profiles(id) on delete cascade,
  to_user_id uuid not null references public.profiles(id) on delete cascade,
  status text not null default 'pending' check (status in ('pending', 'accepted', 'rejected', 'cancelled')),
  created_at timestamptz not null default now(),
  responded_at timestamptz,
  constraint friend_requests_no_self check (from_user_id <> to_user_id)
);
create index idx_friend_requests_from_user_id on public.friend_requests(from_user_id);
create index idx_friend_requests_to_user_id on public.friend_requests(to_user_id);
create unique index friend_requests_unique_pending on public.friend_requests (least(from_user_id, to_user_id), greatest(from_user_id, to_user_id)) where status = 'pending';

create or replace function private.handle_friend_request_accepted()
returns trigger language plpgsql security definer set search_path = public as $$
begin
  if new.status = 'accepted' and old.status is distinct from 'accepted' then
    insert into public.friendships (user_a_id, user_b_id)
    values (least(new.from_user_id, new.to_user_id), greatest(new.from_user_id, new.to_user_id))
    on conflict do nothing;
    new.responded_at = coalesce(new.responded_at, now());
  elsif new.status in ('rejected', 'cancelled') and old.status is distinct from new.status then
    new.responded_at = coalesce(new.responded_at, now());
  end if;
  return new;
end;
$$;
create trigger friend_requests_after_update before update on public.friend_requests for each row execute function private.handle_friend_request_accepted();

create table public.daily_statuses (
  user_id uuid not null references public.profiles(id) on delete cascade,
  status_date date not null default current_date,
  status text not null default 'unselected' check (status in ('unselected', 'available', 'maybe_available', 'depends_on_time', 'has_plans')),
  updated_at timestamptz not null default now(),
  primary key (user_id, status_date)
);

create table public.friend_groups (
  id uuid primary key default gen_random_uuid(),
  owner_user_id uuid not null references public.profiles(id) on delete cascade,
  client_id text not null,
  name text not null,
  sort_order integer not null default 0,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (owner_user_id, client_id),
  constraint friend_groups_client_id_format check (client_id ~ '^[A-Za-z0-9_-]{1,64}$'),
  constraint friend_groups_name_length check (char_length(name) between 1 and 24)
);
create index friend_groups_owner_sort_idx on public.friend_groups(owner_user_id, sort_order, created_at);

create table public.friend_group_members (
  group_id uuid not null references public.friend_groups(id) on delete cascade,
  friend_user_id uuid not null references public.profiles(id) on delete cascade,
  sort_order integer not null default 0,
  created_at timestamptz not null default now(),
  primary key (group_id, friend_user_id)
);
create index friend_group_members_friend_idx on public.friend_group_members(friend_user_id, created_at desc);

create table public.user_blocks (
  blocker_user_id uuid not null references public.profiles(id) on delete cascade,
  blocked_user_id uuid not null references public.profiles(id) on delete cascade,
  reason text,
  created_at timestamptz not null default now(),
  primary key (blocker_user_id, blocked_user_id),
  constraint user_blocks_no_self check (blocker_user_id <> blocked_user_id)
);
create index user_blocks_blocked_idx on public.user_blocks(blocked_user_id, created_at desc);

create table public.user_mutes (
  muter_user_id uuid not null references public.profiles(id) on delete cascade,
  muted_user_id uuid not null references public.profiles(id) on delete cascade,
  created_at timestamptz not null default now(),
  primary key (muter_user_id, muted_user_id),
  constraint user_mutes_no_self check (muter_user_id <> muted_user_id)
);
create index user_mutes_muted_idx on public.user_mutes(muted_user_id, created_at desc);

create table public.user_reports (
  id uuid primary key default gen_random_uuid(),
  reporter_user_id uuid not null references public.profiles(id) on delete cascade,
  reported_user_id uuid not null references public.profiles(id) on delete cascade,
  reason text not null default 'other',
  status text not null default 'pending',
  reviewed_at timestamptz,
  reviewed_by_user_id uuid references public.profiles(id) on delete set null,
  moderation_note text,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (reporter_user_id, reported_user_id),
  constraint user_reports_no_self check (reporter_user_id <> reported_user_id),
  constraint user_reports_reason_check check (reason in ('spam', 'harassment', 'inappropriate', 'violence', 'minor_safety', 'other')),
  constraint user_reports_status_check check (status in ('pending', 'reviewing', 'resolved', 'dismissed'))
);
create index user_reports_status_created_at_idx on public.user_reports(status, created_at desc);
create index user_reports_reported_created_at_idx on public.user_reports(reported_user_id, created_at desc);

create table public.wish_items (
  id uuid primary key default gen_random_uuid(),
  owner_user_id uuid not null references public.profiles(id) on delete cascade,
  title text not null,
  note text not null default '',
  category text not null default 'other' check (category in ('food','drink','cafe','sauna','work','walk','drive','event','other')),
  place_text text not null default '',
  place_url text not null default '',
  visibility text not null default 'private' check (visibility in ('private','friends','group')),
  status text not null default 'active' check (status in ('active','archived','done')),
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);
create index wish_items_owner_created_idx on public.wish_items(owner_user_id, created_at desc);

create table public.yurubos (
  id uuid primary key default gen_random_uuid(),
  owner_user_id uuid not null references public.profiles(id) on delete cascade,
  title text not null,
  body text not null default '',
  category text not null default 'other' check (category in ('food','drink','cafe','sauna','work','walk','drive','event','other')),
  place_text text not null default '',
  place_lat double precision,
  place_lng double precision,
  time_label text not null default '',
  starts_at timestamptz,
  ends_at timestamptz,
  status text not null default 'open' check (status in ('open','closed','expired','cancelled','scheduled')),
  visibility text not null default 'friends' check (visibility in ('friends','group','private')),
  wish_item_id uuid references public.wish_items(id) on delete set null,
  expires_at timestamptz default (now() + interval '30 days'),
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);
create index yurubos_owner_created_idx on public.yurubos(owner_user_id, created_at desc);
create index yurubos_status_expires_idx on public.yurubos(status, expires_at);
create index yurubos_wish_item_idx on public.yurubos(wish_item_id);

create table public.yurubo_reactions (
  id uuid primary key default gen_random_uuid(),
  yurubo_id uuid not null references public.yurubos(id) on delete cascade,
  user_id uuid not null references public.profiles(id) on delete cascade,
  reaction_type text not null default 'interested' check (reaction_type in ('interested','available','another_day')),
  message text not null default '',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (yurubo_id, user_id)
);
create index yurubo_reactions_yurubo_idx on public.yurubo_reactions(yurubo_id);

create table public.hidden_yurubos (
  id uuid primary key default gen_random_uuid(),
  yurubo_id uuid not null references public.yurubos(id) on delete cascade,
  user_id uuid not null references public.profiles(id) on delete cascade,
  created_at timestamptz not null default now(),
  unique (yurubo_id, user_id)
);
create index hidden_yurubos_user_idx on public.hidden_yurubos(user_id);

create table public.yurubo_visibility_groups (
  id uuid primary key default gen_random_uuid(),
  yurubo_id uuid not null references public.yurubos(id) on delete cascade,
  group_id uuid not null references public.friend_groups(id) on delete cascade,
  created_at timestamptz not null default now(),
  unique (yurubo_id, group_id)
);
create index yurubo_visibility_groups_yurubo_idx on public.yurubo_visibility_groups(yurubo_id);
create index yurubo_visibility_groups_group_idx on public.yurubo_visibility_groups(group_id);

create table public.invites (
  id uuid primary key default gen_random_uuid(),
  inviter_user_id uuid not null references public.profiles(id) on delete cascade,
  invitee_user_id uuid not null references public.profiles(id) on delete cascade,
  scheduled_date date not null default current_date,
  activity_label text,
  status text not null default 'pending' check (status in ('pending', 'accepted', 'rejected', 'cancelled')),
  created_at timestamptz not null default now(),
  responded_at timestamptz,
  constraint invites_no_self check (inviter_user_id <> invitee_user_id),
  constraint invites_activity_label_length check (activity_label is null or char_length(activity_label) between 1 and 40)
);
create index invites_inviter_user_idx on public.invites(inviter_user_id, scheduled_date desc, status);
create index invites_invitee_user_idx on public.invites(invitee_user_id, scheduled_date desc, status);
create unique index invites_unique_active_day on public.invites (least(inviter_user_id, invitee_user_id), greatest(inviter_user_id, invitee_user_id), scheduled_date) where status in ('pending', 'accepted');

create table public.notifications (
  id uuid primary key default gen_random_uuid(),
  recipient_user_id uuid not null references public.profiles(id) on delete cascade,
  actor_user_id uuid references public.profiles(id) on delete set null,
  kind text not null,
  title text not null,
  message text not null,
  friend_request_id uuid references public.friend_requests(id) on delete cascade,
  memory_id uuid,
  invite_id uuid references public.invites(id) on delete cascade,
  notification_date date,
  system_key text,
  created_at timestamptz not null default now(),
  read_at timestamptz,
  constraint notifications_no_self_actor check (actor_user_id is null or actor_user_id <> recipient_user_id),
  constraint notifications_kind_check check (kind in ('friend_request_received','friend_request_accepted','invite_received','invite_accepted','today_reservation_reminder','yurubo_created','system'))
);
create index notifications_recipient_created_at_idx on public.notifications(recipient_user_id, created_at desc);
create index notifications_recipient_unread_idx on public.notifications(recipient_user_id, created_at desc) where read_at is null;
create unique index notifications_unique_friend_request_event on public.notifications(recipient_user_id, friend_request_id, kind) where friend_request_id is not null and kind in ('friend_request_received', 'friend_request_accepted');
create unique index notifications_unique_invite_event on public.notifications(recipient_user_id, invite_id, kind) where invite_id is not null and kind in ('invite_received', 'invite_accepted');
create unique index notifications_unique_today_reservation_reminder on public.notifications(recipient_user_id, invite_id, notification_date, kind) where invite_id is not null and notification_date is not null and kind = 'today_reservation_reminder';
create unique index notifications_unique_system_key on public.notifications(recipient_user_id, system_key, kind) where system_key is not null and kind = 'system';
create unique index notifications_unique_yurubo_created on public.notifications(recipient_user_id, system_key, kind) where system_key is not null and kind = 'yurubo_created';

create table public.push_tokens (
  token text primary key,
  user_id uuid not null references public.profiles(id) on delete cascade,
  platform text not null check (platform in ('ios', 'android')),
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  last_seen_at timestamptz not null default now()
);
create index push_tokens_user_id_idx on public.push_tokens(user_id);

create table public.notification_outbox (
  id uuid primary key default gen_random_uuid(),
  event_kind text not null,
  aggregate_type text not null,
  aggregate_id uuid,
  actor_user_id uuid references public.profiles(id) on delete set null,
  recipient_user_id uuid references public.profiles(id) on delete set null,
  payload jsonb not null default '{}'::jsonb,
  status text not null default 'pending',
  attempts integer not null default 0,
  last_error text,
  next_attempt_at timestamptz,
  processed_at timestamptz,
  created_at timestamptz not null default now(),
  constraint notification_outbox_status_check check (status in ('pending', 'processing', 'processed', 'failed')),
  constraint notification_outbox_attempts_check check (attempts >= 0)
);
create index notification_outbox_status_next_attempt_idx on public.notification_outbox(status, next_attempt_at, created_at);
create index notification_outbox_event_idx on public.notification_outbox(event_kind, created_at desc);

create table public.app_schema_migrations (
  version text primary key,
  name text not null,
  applied_at timestamptz not null default now()
);
insert into public.app_schema_migrations(version,name) values ('20260609_neon_baseline','Ohey Neon baseline') on conflict do nothing;

commit;
