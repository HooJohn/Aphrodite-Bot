package models

import (
	"time"
)

// ChatMessage 聊天消息模型
type ChatMessage struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	UserID    string    `json:"user_id" gorm:"index"` // 增加 gorm:"index" 提高查询效率
	Role      string    `json:"role"`               // "user", "assistant", "system"
	Name      string    `json:"name"`               // 发送者名称 (用户ID/昵称, 或 AI Agent 的名字)
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// ChatResponse (当前未使用，可以保留或移除)
// type ChatResponse struct {
// 	ID        uint      `json:"id" gorm:"primaryKey"`
// 	MessageID uint      `json:"message_id"`
// 	Content   string    `json:"content"`
// 	Timestamp time.Time `json:"timestamp"`
// }