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
