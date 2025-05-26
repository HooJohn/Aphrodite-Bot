package models

import (
	"time"
)

// ChatMessage represents a message in a chat.
type ChatMessage struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	UserID    string    `json:"user_id" gorm:"index"` // Added gorm:"index" for query efficiency
	Role      string    `json:"role"`               // Role of the message sender, e.g., "user", "assistant", "system"
	Name      string    `json:"name"`               // Name of the sender (UserID/nickname, or AI Agent's name)
	Content   string    `json:"content"`            // Content of the message
	Timestamp time.Time `json:"timestamp"`          // Timestamp when the message was recorded
}

// ChatResponse (Currently unused, can be kept for future use or removed if deemed unnecessary)
// type ChatResponse struct {
// 	ID        uint      `json:"id" gorm:"primaryKey"`
// 	MessageID uint      `json:"message_id"` // ID of the original ChatMessage this might be a response to
// 	Content   string    `json:"content"`    // Content of the response
// 	Timestamp time.Time `json:"timestamp"`  // Timestamp of the response
// }