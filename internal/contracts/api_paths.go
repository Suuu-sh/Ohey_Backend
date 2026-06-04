package contracts

const (
	APIPathHealth      = "/healthz"
	APIPathShareYurubo = "/share/yurubos/{id}"

	APIPathAuthSignup       = "/v1/auth/signup"
	APIPathMeProfile        = "/v1/me/profile"
	APIPathMeAccount        = "/v1/me/account"
	APIPathMePushToken      = "/v1/me/push-token"
	APIPathProfileByUserID  = "/v1/profiles/by-user-id/{user_id}"
	APIPathFriends          = "/v1/friends"
	APIPathFriend           = "/v1/friends/{id}"
	APIPathFriendFavorite   = "/v1/friends/{id}/favorite"
	APIPathFriendMonthStats = "/v1/friends/{id}/daily-statuses/month"
	APIPathFriendGroups     = "/v1/friend-groups"
	APIPathFriendRequests   = "/v1/friend-requests"
	APIPathFriendReqStatus  = "/v1/friend-requests/status"
	APIPathFriendRequest    = "/v1/friend-requests/{id}"

	APIPathHomeFeed = "/v1/home/feed"

	APIPathWishItems        = "/v1/wish-items"
	APIPathWishItem         = "/v1/wish-items/{id}"
	APIPathProfileWishItems = "/v1/wish-items/profile/{id}"

	APIPathYurubos                = "/v1/yurubos"
	APIPathYurubo                 = "/v1/yurubos/{id}"
	APIPathYuruboReaction         = "/v1/yurubos/{id}/reaction"
	APIPathYuruboReactionApproval = "/v1/yurubos/{id}/reactions/{user_id}"

	APIPathMemories     = "/v1/memories"
	APIPathMemory       = "/v1/memories/{id}"
	APIPathMemoryLike   = "/v1/memories/{id}/like"
	APIPathMemoryReport = "/v1/memories/{id}/report"
	APIPathMemoryHides  = "/v1/memory-hides"
	APIPathMemoryHide   = "/v1/memory-hides/{id}"

	APIPathUserBlocks  = "/v1/user-blocks"
	APIPathUserBlock   = "/v1/user-blocks/{id}"
	APIPathUserMutes   = "/v1/user-mutes"
	APIPathUserMute    = "/v1/user-mutes/{id}"
	APIPathUserReports = "/v1/user-reports"

	APIPathNotifications        = "/v1/notifications"
	APIPathNotificationsReadAll = "/v1/notifications/read-all"

	APIPathDailyStatus            = "/v1/daily-status"
	APIPathMonthlyDailyStatuses   = "/v1/daily-statuses/month"
	APIPathTodayReservations      = "/v1/invites/today-reservations"
	APIPathIncomingPendingInvites = "/v1/invites/incoming-pending"
	APIPathOutgoingActiveInvites  = "/v1/invites/outgoing-active"
	APIPathInvites                = "/v1/invites"
	APIPathInvite                 = "/v1/invites/{id}"

	APIPathAdminMe                        = "/v1/admin/me"
	APIPathAdminUsers                     = "/v1/admin/users"
	APIPathAdminUser                      = "/v1/admin/users/{id}"
	APIPathAdminYurubos                   = "/v1/admin/yurubos"
	APIPathAdminYurubo                    = "/v1/admin/yurubos/{id}"
	APIPathAdminMemories                  = "/v1/admin/memories"
	APIPathAdminMemory                    = "/v1/admin/memories/{id}"
	APIPathAdminMemoryReports             = "/v1/admin/memory-reports"
	APIPathAdminMemoryReport              = "/v1/admin/memory-reports/{id}"
	APIPathAdminNotificationOutbox        = "/v1/admin/notification-outbox"
	APIPathAdminNotificationOutboxProcess = "/v1/admin/notification-outbox/process"
	APIPathAdminNotifications             = "/v1/admin/notifications"
)
