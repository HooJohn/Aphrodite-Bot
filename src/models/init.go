package models

import "project/config"

// InitResponse defines the structure for the /api/init endpoint response.
type InitResponse struct {
	UserType       string                   `json:"user_type"` // "guest" or "registered"
	UserID         string                   `json:"user_id"`
	GuestChatQuota int                      `json:"guest_chat_quota"` // Max quota for guests
	MessagesSent   int                      `json:"messages_sent"`    // Current messages sent by guest (if applicable)
	RemainingQuota int                      `json:"remaining_quota"`  // Calculated remaining quota (if guest)
	Models         map[string]string        `json:"models"`
	Groups         []*config.LLMGroup       `json:"groups"`
	Characters     []*config.LLMCharacter   `json:"characters"`
}
