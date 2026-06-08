package httpapi

type Profile struct {
	ID           string `json:"id"`
	UserID       string `json:"user_id"`
	DisplayName  string `json:"display_name"`
	CharacterKey string `json:"character_key"`
	AvatarURL    string `json:"avatar_url,omitempty"`
	IsPlus       bool   `json:"is_plus"`
	Status       string `json:"status"`
}

type Friend struct {
	ID           string `json:"id"`
	UserID       string `json:"user_id"`
	DisplayName  string `json:"display_name"`
	CharacterKey string `json:"character_key"`
	AvatarURL    string `json:"avatar_url,omitempty"`
	IsPlus       bool   `json:"is_plus"`
}

type DailyStatusRequest struct {
	StatusDate string `json:"status_date"`
	Status     string `json:"status"`
}

type FriendFavoriteRequest struct {
	IsFavorite bool `json:"is_favorite"`
}

type AuthUser struct {
	ID           string         `json:"id"`
	Email        string         `json:"email"`
	AppMetadata  map[string]any `json:"app_metadata"`
	UserMetadata map[string]any `json:"user_metadata"`
}

type AdminCreateUserRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	UserID      string `json:"user_id"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url"`
	Status      string `json:"status"`
	StatusDate  string `json:"status_date"`
	IsPlus      bool   `json:"is_plus"`
}

type AdminUpdateUserRequest struct {
	Email       *string `json:"email"`
	Password    *string `json:"password"`
	UserID      *string `json:"user_id"`
	DisplayName *string `json:"display_name"`
	AvatarURL   *string `json:"avatar_url"`
	Status      *string `json:"status"`
	StatusDate  *string `json:"status_date"`
	IsPlus      *bool   `json:"is_plus"`
}

type AdminCreateYuruboRequest struct {
	OwnerUserID string `json:"owner_user_id"`
	Title       string `json:"title"`
	Body        string `json:"body"`
	Category    string `json:"category"`
	PlaceText   string `json:"place_text"`
	TimeLabel   string `json:"time_label"`
	StartsAt    string `json:"starts_at"`
	Status      string `json:"status"`
	Visibility  string `json:"visibility"`
}

type AdminUpdateYuruboRequest struct {
	OwnerUserID *string `json:"owner_user_id"`
	Title       *string `json:"title"`
	Body        *string `json:"body"`
	Category    *string `json:"category"`
	PlaceText   *string `json:"place_text"`
	TimeLabel   *string `json:"time_label"`
	StartsAt    *string `json:"starts_at"`
	Status      *string `json:"status"`
	Visibility  *string `json:"visibility"`
}

type AdminCreateSystemNotificationRequest struct {
	Title            string   `json:"title"`
	Message          string   `json:"message"`
	RecipientUserIDs []string `json:"recipient_user_ids"`
	SendToAll        bool     `json:"send_to_all"`
	SystemKey        string   `json:"system_key"`
}
