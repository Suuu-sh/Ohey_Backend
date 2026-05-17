package httpapi

import "time"

type Profile struct {
	ID           string `json:"id"`
	UserID       string `json:"user_id"`
	DisplayName  string `json:"display_name"`
	CharacterKey string `json:"character_key"`
	AvatarURL    string `json:"avatar_url,omitempty"`
	IsPlus       bool   `json:"is_plus"`
}

type Friend struct {
	ID           string `json:"id"`
	UserID       string `json:"user_id"`
	DisplayName  string `json:"display_name"`
	CharacterKey string `json:"character_key"`
	AvatarURL    string `json:"avatar_url,omitempty"`
	IsPlus       bool   `json:"is_plus"`
}

type DrinkLog struct {
	ID        string    `json:"id"`
	DrankAt   time.Time `json:"drank_at"`
	PlaceName string    `json:"place_name,omitempty"`
	Memo      string    `json:"memo,omitempty"`
	PhotoPath string    `json:"photo_path,omitempty"`
}

type CreateDrinkLogRequest struct {
	DrankAt   *time.Time `json:"drank_at"`
	PlaceName string     `json:"place_name"`
	Memo      string     `json:"memo"`
	PhotoPath string     `json:"photo_path"`
	FriendIDs []string   `json:"friend_ids"`
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
	IsPlus      bool   `json:"is_plus"`
}

type AdminUpdateUserRequest struct {
	Email       *string `json:"email"`
	Password    *string `json:"password"`
	UserID      *string `json:"user_id"`
	DisplayName *string `json:"display_name"`
	AvatarURL   *string `json:"avatar_url"`
	IsPlus      *bool   `json:"is_plus"`
}

type AdminCreateDrinkLogRequest struct {
	OwnerUserID string    `json:"owner_user_id"`
	DrankAt     time.Time `json:"drank_at"`
	PlaceName   string    `json:"place_name"`
	Memo        string    `json:"memo"`
	PhotoPath   string    `json:"photo_path"`
	FriendIDs   []string  `json:"friend_ids"`
}

type AdminUpdateDrinkLogRequest struct {
	OwnerUserID *string    `json:"owner_user_id"`
	DrankAt     *time.Time `json:"drank_at"`
	PlaceName   *string    `json:"place_name"`
	Memo        *string    `json:"memo"`
	PhotoPath   *string    `json:"photo_path"`
}
