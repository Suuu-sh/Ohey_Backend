package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/yota/ohey/backend/internal/contracts"
	"github.com/yota/ohey/backend/internal/features/notifications"
	"github.com/yota/ohey/backend/internal/supabase"
)

type notificationOutboxEvent struct {
	EventKind       string
	AggregateType   string
	AggregateID     string
	ActorUserID     string
	RecipientUserID string
	Payload         map[string]any
}

type NotificationOutboxProcessResult struct {
	ProcessedCount int `json:"processed_count"`
	FailedCount    int `json:"failed_count"`
	SkippedCount   int `json:"skipped_count"`
}

func ProcessNotificationOutboxOnce(ctx context.Context, deps Dependencies, limit int) (NotificationOutboxProcessResult, error) {
	r := &router{deps: deps}
	return r.processNotificationOutbox(ctx, deps.Config.SupabaseServiceRoleKey, limit)
}

func (r *router) usesPostgresStore() bool {
	return r.deps.Config.DataStore == "postgres" || r.deps.Config.DataStore == "neon"
}

func (r *router) notificationUsecase(_ *http.Request) *notifications.Usecase {
	return notifications.NewUsecase(notifications.Dependencies{
		Repository: r.notificationRepository(),
		PushSender: r.deps.FCM,
		Logger:     r.deps.Logger,
	})
}

func (r *router) notificationRepository() notifications.Repository {
	if r.usesPostgresStore() {
		if r.deps.Postgres == nil {
			return notifications.NewPostgresRepository(nil)
		}
		return notifications.NewPostgresRepository(r.deps.Postgres.Pool())
	}
	return notifications.NewSupabaseRepository(r.deps.Supabase, r.deps.AdminSupabase, r.deps.Config.SupabaseServiceRoleKey)
}

func (r *router) createFriendRequestReceivedNotification(req *http.Request, authToken string, requestRow map[string]any) {
	if err := r.notificationUsecase(req).NotifyFriendRequestReceived(req.Context(), authToken, requestRow); err != nil && r.deps.Logger != nil {
		r.deps.Logger.Warn("failed to dispatch friend request received notification", "error", err)
	}
}

func (r *router) createFriendRequestAcceptedNotification(req *http.Request, authToken string, requestRow map[string]any) {
	if err := r.notificationUsecase(req).NotifyFriendRequestAccepted(req.Context(), authToken, requestRow); err != nil && r.deps.Logger != nil {
		r.deps.Logger.Warn("failed to dispatch friend request accepted notification", "error", err)
	}
}

func (r *router) createInviteReceivedNotification(req *http.Request, authToken string, inviteRow map[string]any) {
	if err := r.notificationUsecase(req).NotifyInviteReceived(req.Context(), authToken, inviteRow); err != nil && r.deps.Logger != nil {
		r.deps.Logger.Warn("failed to dispatch invite received notification", "error", err)
	}
}

func (r *router) createInviteAcceptedNotification(req *http.Request, authToken string, inviteRow map[string]any) {
	if err := r.notificationUsecase(req).NotifyInviteAccepted(req.Context(), authToken, inviteRow); err != nil && r.deps.Logger != nil {
		r.deps.Logger.Warn("failed to dispatch invite accepted notification", "error", err)
	}
}

func (r *router) enqueueAndProcessNotificationOutboxEvent(ctx context.Context, authToken string, event notificationOutboxEvent) {
	outboxID := r.recordNotificationOutboxEvent(ctx, event, contracts.StatusPending)
	if outboxID == "" {
		_ = r.dispatchNotificationOutboxEvent(ctx, authToken, event)
		return
	}
	err := r.dispatchNotificationOutboxEvent(ctx, authToken, event)
	if err != nil {
		r.markNotificationOutboxFailed(ctx, outboxID, 1, err)
		return
	}
	r.markNotificationOutboxProcessed(ctx, outboxID)
}

func (r *router) recordNotificationOutboxEvent(ctx context.Context, event notificationOutboxEvent, status string) string {
	if event.EventKind == "" {
		return ""
	}
	if r.usesPostgresStore() {
		return r.recordPostgresNotificationOutboxEvent(ctx, event, status)
	}
	if r.deps.AdminSupabase == nil || r.deps.Config.SupabaseServiceRoleKey == "" {
		return ""
	}
	payload := event.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	status = strings.TrimSpace(status)
	if status == "" {
		status = contracts.StatusPending
	}
	body := map[string]any{
		"event_kind":     event.EventKind,
		"aggregate_type": event.AggregateType,
		"payload":        payload,
		"status":         status,
	}
	if status == contracts.OutboxStatusProcessed {
		body["attempts"] = 1
		body["processed_at"] = time.Now().UTC().Format(time.RFC3339)
	}
	if event.AggregateID != "" {
		body["aggregate_id"] = event.AggregateID
	}
	if event.ActorUserID != "" {
		body["actor_user_id"] = event.ActorUserID
	}
	if event.RecipientUserID != "" {
		body["recipient_user_id"] = event.RecipientUserID
	}
	var rows []map[string]any
	if err := r.deps.AdminSupabase.Post(ctx, r.deps.Config.SupabaseServiceRoleKey, "notification_outbox", nil, body, &rows); err != nil {
		if r.deps.Logger != nil {
			r.deps.Logger.Warn("failed to record notification outbox event", "event", event.EventKind, "error", err)
		}
		return ""
	}
	id, _ := firstMap(rows, nil)["id"].(string)
	return id
}

func (r *router) recordPostgresNotificationOutboxEvent(ctx context.Context, event notificationOutboxEvent, status string) string {
	if r.deps.Postgres == nil {
		return ""
	}
	payload := event.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	status = strings.TrimSpace(status)
	if status == "" {
		status = contracts.StatusPending
	}
	attempts := 0
	var processedAt any
	if status == contracts.OutboxStatusProcessed {
		attempts = 1
		processedAt = time.Now().UTC()
	}
	var id string
	err = r.deps.Postgres.Pool().QueryRow(ctx, `insert into notification_outbox (event_kind,aggregate_type,aggregate_id,actor_user_id,recipient_user_id,payload,status,attempts,processed_at) values ($1,$2,nullif($3,'')::uuid,nullif($4,'')::uuid,nullif($5,'')::uuid,$6::jsonb,$7,$8,$9) returning id::text`, event.EventKind, event.AggregateType, event.AggregateID, event.ActorUserID, event.RecipientUserID, string(payloadJSON), status, attempts, processedAt).Scan(&id)
	if err != nil {
		if r.deps.Logger != nil {
			r.deps.Logger.Warn("failed to record notification outbox event", "event", event.EventKind, "error", err)
		}
		return ""
	}
	return id
}

func (r *router) dispatchNotificationOutboxEvent(ctx context.Context, authToken string, event notificationOutboxEvent) error {
	usecase := notifications.NewUsecase(notifications.Dependencies{
		Repository: r.notificationRepository(),
		PushSender: r.deps.FCM,
		Logger:     r.deps.Logger,
	})
	switch event.EventKind {
	case contracts.DomainEventFriendRequestCreated:
		return usecase.NotifyFriendRequestReceived(ctx, authToken, event.Payload)
	case contracts.DomainEventFriendRequestAccepted:
		return usecase.NotifyFriendRequestAccepted(ctx, authToken, event.Payload)
	case contracts.DomainEventInviteCreated:
		return usecase.NotifyInviteReceived(ctx, authToken, event.Payload)
	case contracts.DomainEventInviteAccepted:
		return usecase.NotifyInviteAccepted(ctx, authToken, event.Payload)
	case contracts.DomainEventYuruboCreated:
		return usecase.NotifyYuruboCreated(ctx, authToken, event.Payload, stringSliceValue(event.Payload, "group_ids"))
	case contracts.DomainEventSystemNotificationCreated:
		return nil
	default:
		return fmt.Errorf("unsupported notification outbox event kind: %s", event.EventKind)
	}
}

func (r *router) markNotificationOutboxProcessed(ctx context.Context, outboxID string) {
	r.patchNotificationOutbox(ctx, outboxID, map[string]any{
		"status":       contracts.OutboxStatusProcessed,
		"processed_at": time.Now().UTC().Format(time.RFC3339),
		"last_error":   nil,
	})
}

func (r *router) markNotificationOutboxFailed(ctx context.Context, outboxID string, attempts int, err error) {
	if err == nil {
		return
	}
	if attempts < 0 {
		attempts = 0
	}
	nextAttemptAt := time.Now().UTC().Add(time.Duration(1<<min(attempts, 6)) * time.Minute)
	r.patchNotificationOutbox(ctx, outboxID, map[string]any{
		"status":          contracts.OutboxStatusFailed,
		"attempts":        attempts,
		"last_error":      shortText(err.Error(), 500),
		"next_attempt_at": nextAttemptAt.Format(time.RFC3339),
	})
}

func (r *router) patchNotificationOutbox(ctx context.Context, outboxID string, payload map[string]any) {
	if r.usesPostgresStore() {
		r.patchPostgresNotificationOutbox(ctx, outboxID, payload)
		return
	}
	if r.deps.AdminSupabase == nil || r.deps.Config.SupabaseServiceRoleKey == "" || outboxID == "" {
		return
	}
	q := url.Values{}
	q.Set("id", "eq."+outboxID)
	var ignored []map[string]any
	if err := r.deps.AdminSupabase.Patch(ctx, r.deps.Config.SupabaseServiceRoleKey, "notification_outbox", q, payload, &ignored); err != nil && r.deps.Logger != nil {
		r.deps.Logger.Warn("failed to update notification outbox", "id", outboxID, "error", err)
	}
}

func (r *router) patchPostgresNotificationOutbox(ctx context.Context, outboxID string, payload map[string]any) {
	if r.deps.Postgres == nil || outboxID == "" {
		return
	}
	_, err := r.deps.Postgres.Pool().Exec(ctx, `update notification_outbox set status=coalesce($2,status), attempts=coalesce($3,attempts), last_error=$4, next_attempt_at=$5, processed_at=$6 where id=$1`, outboxID, nullableString(payload["status"]), nullableInt(payload["attempts"]), nullableString(payload["last_error"]), nullableTime(payload["next_attempt_at"]), nullableTime(payload["processed_at"]))
	if err != nil && r.deps.Logger != nil {
		r.deps.Logger.Warn("failed to update notification outbox", "id", outboxID, "error", err)
	}
}

func (r *router) adminListNotificationOutbox(w http.ResponseWriter, req *http.Request, _ AuthUser) {
	if r.usesPostgresStore() {
		rows, err := r.listPostgresNotificationOutbox(req.Context(), strings.TrimSpace(req.URL.Query().Get("status")), 100)
		if err != nil {
			writeError(w, http.StatusBadGateway, "database error")
			return
		}
		writeJSON(w, http.StatusOK, rows)
		return
	}
	q := url.Values{}
	q.Set("select", "id,event_kind,aggregate_type,aggregate_id,actor_user_id,recipient_user_id,status,attempts,last_error,next_attempt_at,processed_at,created_at,payload")
	q.Set("order", "created_at.desc")
	q.Set("limit", "100")
	if status := strings.TrimSpace(req.URL.Query().Get("status")); status != "" && status != contracts.QueryStatusAll {
		q.Set("status", supabase.PostgRESTEq(status))
	}
	var rows []map[string]any
	if err := r.deps.AdminSupabase.Get(req.Context(), r.deps.Config.SupabaseServiceRoleKey, "notification_outbox", q, &rows); err != nil {
		writeSupabaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (r *router) adminProcessNotificationOutbox(w http.ResponseWriter, req *http.Request, _ AuthUser) {
	limit := 20
	if rawLimit := strings.TrimSpace(req.URL.Query().Get("limit")); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed <= 0 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		limit = min(parsed, 100)
	}
	result, err := r.processNotificationOutbox(req.Context(), r.deps.Config.SupabaseServiceRoleKey, limit)
	if err != nil {
		writeSupabaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (r *router) processNotificationOutbox(ctx context.Context, authToken string, limit int) (NotificationOutboxProcessResult, error) {
	rows, err := r.notificationOutboxDueRows(ctx, limit)
	if err != nil {
		return NotificationOutboxProcessResult{}, err
	}
	result := NotificationOutboxProcessResult{}
	for _, row := range rows {
		id := stringValue(row, "id")
		event := notificationOutboxEvent{
			EventKind:       stringValue(row, "event_kind"),
			AggregateType:   stringValue(row, "aggregate_type"),
			AggregateID:     stringValue(row, "aggregate_id"),
			ActorUserID:     stringValue(row, "actor_user_id"),
			RecipientUserID: stringValue(row, "recipient_user_id"),
			Payload:         mapValue(row, "payload"),
		}
		attempts := intValue(row, "attempts") + 1
		err := r.dispatchNotificationOutboxEvent(ctx, authToken, event)
		if err != nil {
			result.FailedCount++
			r.markNotificationOutboxFailed(ctx, id, attempts, err)
			continue
		}
		result.ProcessedCount++
		r.patchNotificationOutbox(ctx, id, map[string]any{
			"status":       contracts.OutboxStatusProcessed,
			"attempts":     attempts,
			"processed_at": time.Now().UTC().Format(time.RFC3339),
			"last_error":   nil,
		})
	}
	return result, nil
}

func (r *router) notificationOutboxDueRows(ctx context.Context, limit int) ([]map[string]any, error) {
	if r.usesPostgresStore() {
		return r.postgresNotificationOutboxDueRows(ctx, limit)
	}
	if r.deps.AdminSupabase == nil || r.deps.Config.SupabaseServiceRoleKey == "" {
		return []map[string]any{}, nil
	}
	q := url.Values{}
	q.Set("select", "id,event_kind,aggregate_type,aggregate_id,actor_user_id,recipient_user_id,status,attempts,payload,next_attempt_at")
	q.Set("status", supabase.PostgRESTIn(contracts.StatusPending, contracts.OutboxStatusFailed))
	q.Set("order", "created_at.asc")
	q.Set("limit", strconv.Itoa(limit))
	var rows []map[string]any
	if err := r.deps.AdminSupabase.Get(ctx, r.deps.Config.SupabaseServiceRoleKey, "notification_outbox", q, &rows); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	due := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		nextAttempt := strings.TrimSpace(stringValue(row, "next_attempt_at"))
		if nextAttempt == "" {
			due = append(due, row)
			continue
		}
		parsed, err := time.Parse(time.RFC3339, nextAttempt)
		if err != nil || !parsed.After(now) {
			due = append(due, row)
		}
	}
	return due, nil
}

func (r *router) listPostgresNotificationOutbox(ctx context.Context, status string, limit int) ([]map[string]any, error) {
	if r.deps.Postgres == nil {
		return []map[string]any{}, nil
	}
	if limit <= 0 {
		limit = 100
	}
	where := ""
	args := []any{limit}
	if status != "" && status != contracts.QueryStatusAll {
		where = " where status=$2"
		args = append(args, status)
	}
	rows, err := r.deps.Postgres.Pool().Query(ctx, `select id::text,event_kind,aggregate_type,aggregate_id::text,actor_user_id::text,recipient_user_id::text,status,attempts,last_error,next_attempt_at,processed_at,created_at,payload from notification_outbox`+where+` order by created_at desc limit $1`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		m, err := scanNotificationOutboxRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *router) postgresNotificationOutboxDueRows(ctx context.Context, limit int) ([]map[string]any, error) {
	if r.deps.Postgres == nil {
		return []map[string]any{}, nil
	}
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.deps.Postgres.Pool().Query(ctx, `select id::text,event_kind,aggregate_type,aggregate_id::text,actor_user_id::text,recipient_user_id::text,status,attempts,last_error,next_attempt_at,processed_at,created_at,payload from notification_outbox where status=any($2) and (next_attempt_at is null or next_attempt_at<=now()) order by created_at asc limit $1`, limit, []string{contracts.StatusPending, contracts.OutboxStatusFailed})
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		m, err := scanNotificationOutboxRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func stringValue(row map[string]any, key string) string {
	value, _ := row[key].(string)
	return value
}

func intValue(row map[string]any, key string) int {
	switch v := row[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case int32:
		return int(v)
	case float64:
		return int(v)
	case float32:
		return int(v)
	default:
		return 0
	}
}

func mapValue(row map[string]any, key string) map[string]any {
	if value, ok := row[key].(map[string]any); ok {
		return value
	}
	if value, ok := row[key].(map[string]interface{}); ok {
		return value
	}
	return map[string]any{}
}

func scanNotificationOutboxRow(row interface{ Scan(dest ...any) error }) (map[string]any, error) {
	var id, eventKind, aggregateType, status string
	var aggregateID, actorUserID, recipientUserID, lastError *string
	var attempts int
	var nextAttemptAt, processedAt *time.Time
	var createdAt time.Time
	var payloadBytes []byte
	if err := row.Scan(&id, &eventKind, &aggregateType, &aggregateID, &actorUserID, &recipientUserID, &status, &attempts, &lastError, &nextAttemptAt, &processedAt, &createdAt, &payloadBytes); err != nil {
		return nil, err
	}
	payload := map[string]any{}
	if len(payloadBytes) > 0 {
		_ = json.Unmarshal(payloadBytes, &payload)
	}
	m := map[string]any{"id": id, "event_kind": eventKind, "aggregate_type": aggregateType, "status": status, "attempts": attempts, "created_at": createdAt.UTC().Format(time.RFC3339Nano), "payload": payload}
	if aggregateID != nil {
		m["aggregate_id"] = *aggregateID
	}
	if actorUserID != nil {
		m["actor_user_id"] = *actorUserID
	}
	if recipientUserID != nil {
		m["recipient_user_id"] = *recipientUserID
	}
	if lastError != nil {
		m["last_error"] = *lastError
	}
	if nextAttemptAt != nil {
		m["next_attempt_at"] = nextAttemptAt.UTC().Format(time.RFC3339Nano)
	}
	if processedAt != nil {
		m["processed_at"] = processedAt.UTC().Format(time.RFC3339Nano)
	}
	return m, nil
}

func nullableString(v any) any {
	if v == nil {
		return nil
	}
	if s, ok := v.(string); ok {
		return s
	}
	return nil
}

func nullableInt(v any) any {
	if v == nil {
		return nil
	}
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return n
	case float64:
		return int(n)
	}
	return nil
}

func nullableTime(v any) any {
	if v == nil {
		return nil
	}
	if t, ok := v.(time.Time); ok {
		return t
	}
	if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t
		}
		if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
			return t
		}
	}
	return nil
}

func stringSliceValue(row map[string]any, key string) []string {
	raw, ok := row[key].([]any)
	if !ok {
		if values, ok := row[key].([]string); ok {
			return values
		}
		return nil
	}
	values := make([]string, 0, len(raw))
	for _, item := range raw {
		if value, ok := item.(string); ok && strings.TrimSpace(value) != "" {
			values = append(values, strings.TrimSpace(value))
		}
	}
	return values
}

func (r *router) adminCreateNotification(w http.ResponseWriter, req *http.Request, _ AuthUser) {
	var input AdminCreateSystemNotificationRequest
	if !decodeJSONBody(w, req, &input) {
		return
	}
	result, err := r.notificationUsecase(req).CreateSystemNotifications(req.Context(), notifications.CreateSystemInput{
		Title:            input.Title,
		Message:          input.Message,
		RecipientUserIDs: input.RecipientUserIDs,
		SendToAll:        input.SendToAll,
		SystemKey:        input.SystemKey,
	})
	if err != nil {
		writeNotificationError(w, err)
		return
	}
	r.recordNotificationOutboxEvent(req.Context(), notificationOutboxEvent{
		EventKind:     contracts.DomainEventSystemNotificationCreated,
		AggregateType: "system_notification",
		Payload: map[string]any{
			"title":              input.Title,
			"message":            input.Message,
			"recipient_user_ids": input.RecipientUserIDs,
			"send_to_all":        input.SendToAll,
			"system_key":         input.SystemKey,
			"recipient_count":    result.RecipientCount,
			"created_count":      result.CreatedCount,
		},
	}, contracts.OutboxStatusProcessed)
	writeJSON(w, http.StatusCreated, result)
}

func writeNotificationError(w http.ResponseWriter, err error) {
	if kind, ok := notifications.ErrorKindOf(err); ok {
		switch kind {
		case notifications.ErrorKindInvalidInput:
			writeError(w, http.StatusBadRequest, err.Error())
		case notifications.ErrorKindUpstream:
			writeError(w, http.StatusBadGateway, "upstream service error")
		default:
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	writeSupabaseError(w, err)
}
