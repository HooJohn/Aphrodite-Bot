package repository

import (
	"errors"
	"fmt"
	"log"
	"sync"

	"project/models"
)

// ChatRepository defines the interface for chat message storage.
type ChatRepository interface {
	SaveMessage(message models.ChatMessage) error
	GetMessagesByUserID(userID string) ([]models.ChatMessage, error)
}

// chatRepository is an in-memory implementation of ChatRepository.
// It stores messages indexed by UserID for efficient retrieval.
type chatRepository struct {
	messages map[string][]models.ChatMessage // Stores lists of messages per UserID
	mu       sync.RWMutex
	// nextMessageID uint // If a globally unique message ID (across users) were needed, this could be used.
	// Currently, models.ChatMessage.ID is a uint, and SaveMessage handles making it unique within the user's message list.
}

// NewChatRepository creates an instance of the in-memory chat repository.
func NewChatRepository() ChatRepository {
	return &chatRepository{
		messages: make(map[string][]models.ChatMessage),
	}
}

// SaveMessage saves a chat message to the in-memory store.
// It assigns a new ID to the message, specific to the user's message sequence.
func (r *chatRepository) SaveMessage(message models.ChatMessage) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if message.UserID == "" {
		log.Println("ERROR: [ChatRepository] SaveMessage: UserID cannot be empty.")
		return errors.New("failed to save message: UserID cannot be empty")
	}

	userMessages := r.messages[message.UserID]
	message.ID = uint(len(userMessages) + 1) // Assign ID incrementally within the user's message list
	
	r.messages[message.UserID] = append(userMessages, message)

	log.Printf("INFO: [ChatRepository] Message saved to in-memory store: UserID=%s, MsgID=%d, Role=%s, Name=%s, Content='%.30s...'", 
		message.UserID, message.ID, message.Role, message.Name, message.Content)
	return nil
}

// GetMessagesByUserID retrieves all messages for a given user ID from the in-memory store.
// It returns an empty slice and no error if the user has no messages or does not exist.
func (r *chatRepository) GetMessagesByUserID(userID string) ([]models.ChatMessage, error) {
	if userID == "" {
		log.Println("WARN: [ChatRepository] GetMessagesByUserID: UserID cannot be empty.")
		// Returning error for empty userID, though service layer should ideally prevent this.
		return nil, errors.New("userID cannot be empty when fetching messages")
	}
	
	r.mu.RLock()
	defer r.mu.RUnlock()

	userMessages, exists := r.messages[userID]
	if !exists || len(userMessages) == 0 {
		// This behavior (empty slice, nil error for not found) is intentional.
		// It indicates either the user exists with no messages, or the user has no record yet.
		// Service layer can interpret this as "no history".
		log.Printf("INFO: [ChatRepository] No chat history found for UserID '%s' in in-memory store, or user does not exist.", userID)
		return []models.ChatMessage{}, nil // Return empty slice, not an error
	}

	// Return a copy to prevent external modification of the internal store
	result := make([]models.ChatMessage, len(userMessages))
	copy(result, userMessages)

	log.Printf("INFO: [ChatRepository] Retrieved %d messages for UserID '%s' from in-memory store.", len(result), userID)
	return result, nil
}