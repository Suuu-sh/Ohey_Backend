package media

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"time"
)

type ErrorKind string

const (
	ErrorKindInvalidInput ErrorKind = "invalid_input"
	ErrorKindUpstream     ErrorKind = "upstream"
)

type UserError struct {
	Kind    ErrorKind
	Message string
}

func (e UserError) Error() string {
	return e.Message
}

func ErrorKindOf(err error) (ErrorKind, bool) {
	var userErr UserError
	if errors.As(err, &userErr) {
		return userErr.Kind, true
	}
	return "", false
}

var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func CleanUUID(value, field string) (string, error) {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	if trimmed == "" {
		return "", UserError{Kind: ErrorKindInvalidInput, Message: field + " is required"}
	}
	if !uuidPattern.MatchString(trimmed) {
		return "", UserError{Kind: ErrorKindInvalidInput, Message: field + " must be a valid UUID"}
	}
	return trimmed, nil
}

type UploadKind string

const (
	UploadKindDrinkLogPhoto UploadKind = "drink_log_photo"
	PhotoBucket                        = "nomo-photos"
)

type UploadRequest struct {
	Kind          string
	UserID        string
	ContentType   string
	FileExtension string
}

type UploadTarget struct {
	Kind        UploadKind
	UserID      string
	Bucket      string
	Path        string
	ContentType string
}

type UploadURL struct {
	Bucket      string `json:"bucket"`
	Path        string `json:"path"`
	Token       string `json:"token"`
	SignedURL   string `json:"signed_url"`
	ContentType string `json:"content_type"`
}

func NewUploadTarget(input UploadRequest, now time.Time, randomSuffix func() string) (UploadTarget, error) {
	userID, err := CleanUUID(input.UserID, "user id")
	if err != nil {
		return UploadTarget{}, err
	}
	kind := UploadKind(strings.TrimSpace(input.Kind))
	if kind == "" {
		kind = UploadKindDrinkLogPhoto
	}
	if kind != UploadKindDrinkLogPhoto {
		return UploadTarget{}, UserError{Kind: ErrorKindInvalidInput, Message: "kind is unsupported"}
	}
	extension, contentType, err := cleanPhotoType(input.FileExtension, input.ContentType)
	if err != nil {
		return UploadTarget{}, err
	}
	if randomSuffix == nil {
		randomSuffix = RandomSuffix
	}
	suffix := strings.TrimSpace(randomSuffix())
	if suffix == "" {
		suffix = RandomSuffix()
	}
	path := "users/" + userID + "/drink_logs/" + now.UTC().Format("20060102T150405.000000000") + "_" + suffix + extension
	return UploadTarget{Kind: kind, UserID: userID, Bucket: PhotoBucket, Path: path, ContentType: contentType}, nil
}

func cleanPhotoType(fileExtension, contentType string) (string, string, error) {
	extension := strings.ToLower(strings.TrimSpace(fileExtension))
	if extension != "" && !strings.HasPrefix(extension, ".") {
		extension = "." + extension
	}
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	if contentType == "image/jpg" {
		contentType = "image/jpeg"
	}
	byExtension := map[string]string{
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".png":  "image/png",
		".heic": "image/heic",
		".webp": "image/webp",
	}
	byContentType := map[string]string{
		"image/jpeg": ".jpg",
		"image/png":  ".png",
		"image/heic": ".heic",
		"image/webp": ".webp",
	}
	if extension == "" && contentType == "" {
		return ".jpg", "image/jpeg", nil
	}
	if extension != "" {
		mappedContentType, ok := byExtension[extension]
		if !ok {
			return "", "", UserError{Kind: ErrorKindInvalidInput, Message: "file_extension is unsupported"}
		}
		if contentType != "" && contentType != mappedContentType {
			return "", "", UserError{Kind: ErrorKindInvalidInput, Message: "content_type does not match file_extension"}
		}
		return extension, mappedContentType, nil
	}
	extension, ok := byContentType[contentType]
	if !ok {
		return "", "", UserError{Kind: ErrorKindInvalidInput, Message: "content_type is unsupported"}
	}
	return extension, contentType, nil
}

func RandomSuffix() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return hex.EncodeToString([]byte(time.Now().UTC().Format("150405.000000000")))
	}
	return hex.EncodeToString(buf[:])
}
