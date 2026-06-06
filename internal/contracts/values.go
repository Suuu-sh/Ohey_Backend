package contracts

const (
	StatusPending   = "pending"
	StatusAccepted  = "accepted"
	StatusRejected  = "rejected"
	StatusCancelled = "cancelled"

	DailyStatusUnselected     = "unselected"
	DailyStatusAvailable      = "available"
	DailyStatusMaybeAvailable = "maybe_available"
	DailyStatusDependsOnTime  = "depends_on_time"
	DailyStatusHasPlans       = "has_plans"

	StatusActive = "active"
	StatusOpen   = "open"

	ModerationStatusReviewing = "reviewing"
	ModerationStatusResolved  = "resolved"
	ModerationStatusDismissed = "dismissed"

	OutboxStatusProcessed = "processed"
	OutboxStatusFailed    = "failed"
	QueryStatusAll        = "all"
)

const (
	VisibilityPrivate = "private"
	VisibilityFriends = "friends"
	VisibilityGroup   = "group"
)

const (
	CategoryOther = "other"
)

const (
	ReactionTypeAvailable  = DailyStatusAvailable
	ReactionTypeInterested = "interested"
	ReactionTypeAnotherDay = "another_day"
)

const (
	RequestDirectionAll      = "all"
	RequestDirectionIncoming = "incoming"
	RequestDirectionOutgoing = "outgoing"

	RelationshipStateNone     = "none"
	RelationshipStateSelf     = "self"
	RelationshipStateOutgoing = RequestDirectionOutgoing
	RelationshipStateIncoming = RequestDirectionIncoming
)

const (
	ReportReasonSpam          = "spam"
	ReportReasonHarassment    = "harassment"
	ReportReasonInappropriate = "inappropriate"
	ReportReasonViolence      = "violence"
	ReportReasonMinorSafety   = "minor_safety"
	ReportReasonOther         = "other"
)

const (
	NotificationKindFriendRequestReceived    = "friend_request_received"
	NotificationKindFriendRequestAccepted    = "friend_request_accepted"
	NotificationKindInviteReceived           = "invite_received"
	NotificationKindInviteAccepted           = "invite_accepted"
	NotificationKindTodayReservationReminder = "today_reservation_reminder"
	NotificationKindMemoryTagged             = "memory_tagged"
	NotificationKindYuruboCreated            = "yurubo_created"
	NotificationKindSystem                   = "system"
)

const (
	FeedTypeMemory       = "memory"
	FeedPostKindMine     = "mine"
	FeedPostKindFriend   = "friend"
	FeedPostKindOfficial = "official"
	FeedPropMemory       = "memory"
)

const (
	PushPlatformIOS     = "ios"
	PushPlatformAndroid = "android"
)

const (
	DomainEventInviteCreated             = "invite.created"
	DomainEventInviteAccepted            = "invite.accepted"
	DomainEventFriendRequestCreated      = "friend_request.created"
	DomainEventFriendRequestAccepted     = "friend_request.accepted"
	DomainEventMemoryTagged              = "memory.tagged"
	DomainEventMemoryReported            = "memory.reported"
	DomainEventYuruboCreated             = "yurubo.created"
	DomainEventSystemNotificationCreated = "system_notification.created"
)
