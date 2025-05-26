package models

import "time"

// GuestQuota tracks the chat message quota for guest users.
type GuestQuota struct {
	GuestUserID  string    `gorm:"primaryKey"` // Guest's temporary UserID
	MessagesSent int       `gorm:"default:0"`
	CreatedAt    time.Time // Automatically managed by GORM
	UpdatedAt    time.Time // Automatically managed by GORM
}

// TableName specifies the table name for GuestQuota model.
func (GuestQuota) TableName() string {
	return "guest_quotas"
}
