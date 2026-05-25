package httpapi

import "time"

type Profile struct {
	ID           string `json:"id"`
	UserID       string `json:"user_id"`
	DisplayName  string `json:"display_name"`
	Gender       string `json:"gender"`
	CharacterKey string `json:"character_key"`
	AvatarURL    string `json:"avatar_url,omitempty"`
	IsPlus       bool   `json:"is_plus"`
	Status       string `json:"status"`
}

type Friend struct {
	ID           string `json:"id"`
	UserID       string `json:"user_id"`
	DisplayName  string `json:"display_name"`
	Gender       string `json:"gender"`
	CharacterKey string `json:"character_key"`
	AvatarURL    string `json:"avatar_url,omitempty"`
	IsPlus       bool   `json:"is_plus"`
}

type DrinkLog struct {
	ID           string    `json:"id"`
	DrankAt      time.Time `json:"drank_at"`
	PlaceName    string    `json:"place_name,omitempty"`
	PlaceLat     *float64  `json:"place_lat,omitempty"`
	PlaceLng     *float64  `json:"place_lng,omitempty"`
	Memo         string    `json:"memo,omitempty"`
	CaptionY     float64   `json:"caption_y"`
	PhotoPath    string    `json:"photo_path,omitempty"`
	LinkURL      string    `json:"link_url,omitempty"`
	MarkerRarity string    `json:"marker_rarity,omitempty"`
	IsOfficial   bool      `json:"is_official"`
}

type CreateDrinkLogRequest struct {
	DrankAt               *time.Time `json:"drank_at"`
	DrankOn               string     `json:"drank_on"`
	TimezoneOffsetMinutes *int       `json:"timezone_offset_minutes"`
	PlaceName             string     `json:"place_name"`
	PlaceLat              *float64   `json:"place_lat"`
	PlaceLng              *float64   `json:"place_lng"`
	Memo                  string     `json:"memo"`
	CaptionY              *float64   `json:"caption_y"`
	PhotoPath             string     `json:"photo_path"`
	MarkerRarity          string     `json:"marker_rarity"`
	FriendIDs             []string   `json:"friend_ids"`
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
	Gender      string `json:"gender"`
	AvatarURL   string `json:"avatar_url"`
	Status      string `json:"status"`
	IsPlus      bool   `json:"is_plus"`
}

type AdminUpdateUserRequest struct {
	Email       *string `json:"email"`
	Password    *string `json:"password"`
	UserID      *string `json:"user_id"`
	DisplayName *string `json:"display_name"`
	Gender      *string `json:"gender"`
	AvatarURL   *string `json:"avatar_url"`
	Status      *string `json:"status"`
	IsPlus      *bool   `json:"is_plus"`
}

type AdminCreateDrinkLogRequest struct {
	OwnerUserID  string    `json:"owner_user_id"`
	DrankAt      time.Time `json:"drank_at"`
	PlaceName    string    `json:"place_name"`
	Memo         string    `json:"memo"`
	CaptionY     *float64  `json:"caption_y"`
	PhotoPath    string    `json:"photo_path"`
	LinkURL      string    `json:"link_url"`
	MarkerRarity string    `json:"marker_rarity"`
	FriendIDs    []string  `json:"friend_ids"`
	IsOfficial   bool      `json:"is_official"`
}

type AdminUpdateDrinkLogRequest struct {
	OwnerUserID  *string    `json:"owner_user_id"`
	DrankAt      *time.Time `json:"drank_at"`
	PlaceName    *string    `json:"place_name"`
	Memo         *string    `json:"memo"`
	CaptionY     *float64   `json:"caption_y"`
	PhotoPath    *string    `json:"photo_path"`
	LinkURL      *string    `json:"link_url"`
	MarkerRarity *string    `json:"marker_rarity"`
	IsOfficial   *bool      `json:"is_official"`
}

type AdminCreateSystemNotificationRequest struct {
	Title            string   `json:"title"`
	Message          string   `json:"message"`
	RecipientUserIDs []string `json:"recipient_user_ids"`
	SendToAll        bool     `json:"send_to_all"`
	SystemKey        string   `json:"system_key"`
}
