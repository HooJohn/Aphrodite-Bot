package models

import "time"

// KnowledgeArticle represents an article in the knowledge base.
type KnowledgeArticle struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Title     string    `gorm:"not null" json:"title"`
	Content   string    `gorm:"type:text" json:"content"`
	Category  string    `gorm:"index" json:"category"` // e.g., "性健康常识" (General Sex Ed), "两性关系" (Relationships), "避孕节育" (Contraception)
	Tags      string    `json:"tags"`                  // Comma-separated tags for searchability
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName specifies the table name for the KnowledgeArticle model.
func (KnowledgeArticle) TableName() string {
	return "knowledge_articles"
}
