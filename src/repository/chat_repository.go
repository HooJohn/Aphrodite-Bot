package repository

import (
	"errors"
	"log"
	"sync"

	"project/models"
)

// ChatRepository 聊天仓库接口 (保持不变)
type ChatRepository interface {
	SaveMessage(message models.ChatMessage) error
	GetMessagesByUserID(userID string) ([]models.ChatMessage, error)
}

// chatRepository 聊天仓库实现
type chatRepository struct {
	messages map[string][]models.ChatMessage // 修改：按 UserID 存储消息列表，提高查询效率
	mu       sync.RWMutex
	// nextMessageID uint // 如果需要全局唯一的 message ID (跨用户)，可以保留。但通常 message ID 在用户级别内自增或由DB生成。
	// 当前 models.ChatMessage.ID 是 uint，我们会在 SaveMessage 中处理它，使其在用户级别内唯一。
}

// NewChatRepository 创建聊天仓库实例
func NewChatRepository() ChatRepository {
	return &chatRepository{
		messages: make(map[string][]models.ChatMessage),
		// nextMessageID: 1,
	}
}

// SaveMessage 保存消息
func (r *chatRepository) SaveMessage(message models.ChatMessage) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if message.UserID == "" {
		return errors.New("保存消息失败：UserID 不能为空")
	}

	userMessages := r.messages[message.UserID]
	message.ID = uint(len(userMessages) + 1) // 在该用户的消息列表内ID自增
	
	r.messages[message.UserID] = append(userMessages, message)

	log.Printf("消息已保存到内存仓库: UserID=%s, MsgID=%d, Role=%s, Name=%s, Content='%.30s...'", message.UserID, message.ID, message.Role, message.Name, message.Content)
	return nil
}

// GetMessagesByUserID 根据用户ID获取消息
func (r *chatRepository) GetMessagesByUserID(userID string) ([]models.ChatMessage, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	userMessages, exists := r.messages[userID]
	if !exists || len(userMessages) == 0 {
		// 返回空切片和nil错误，表示用户存在但没有消息，或用户根本没有记录。
		// ChatHandler 中对 "未找到该用户的消息" 的错误检查需要调整，或者这里返回特定错误。
		// 为了与之前的约定一致，如果希望上层检查特定错误字符串：
		// return nil, errors.New("未找到该用户的消息")
		// 但通常，没有消息不应该是一个错误，而是一个空结果。
		log.Printf("用户 '%s' 没有聊天记录或用户不存在于内存仓库中", userID)
		return []models.ChatMessage{}, nil // 返回空切片，表示没有消息
	}

	// 返回副本以避免外部修改内部存储
	result := make([]models.ChatMessage, len(userMessages))
	copy(result, userMessages)

	log.Printf("从内存仓库为用户 '%s' 获取了 %d 条消息", userID, len(result))
	return result, nil
}