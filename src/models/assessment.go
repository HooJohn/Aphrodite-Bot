package models

import (
	"time"
)

// AssessmentQuestionType 定义了评估问题的类型
type AssessmentQuestionType string

const (
	QuestionTypeSingleChoice AssessmentQuestionType = "single_choice"
	QuestionTypeMultiChoice  AssessmentQuestionType = "multi_choice"
	QuestionTypeOpenText     AssessmentQuestionType = "open_text"
	QuestionTypeConfirmation AssessmentQuestionType = "confirmation"
)

// AssessmentQuestion 定义评估问卷中的一个问题
// 注意：这些问题定义我们当前计划硬编码在 AssessmentService 中，所以这里不需要GORM标签
type AssessmentQuestion struct {
	ID             string                 `json:"id"`
	Order          int                    `json:"order"`
	Text           string                 `json:"text"`
	QuestionType   AssessmentQuestionType `json:"question_type"`
	Options        []string               `json:"options,omitempty"`
	IsRequired     bool                   `json:"is_required"`
	NextQuestionID string                 `json:"next_question_id,omitempty"` // 用于简单固定流程，或在跳转逻辑中备用
	// Category       string                 `json:"category,omitempty"`
	// HelperText     string                 `json:"helper_text,omitempty"`
}

// UserAnswer 代表用户对一个评估问题的回答
type UserAnswer struct {
	QuestionID string   `json:"question_id"`
	Answer     []string `json:"answer"` // 使用切片以支持多选和单选/文本
	// AnsweredAt time.Time `json:"answered_at"` // (可选)
}

// UserAssessmentStatus 定义用户评估的状态
type UserAssessmentStatus string

const (
	AssessmentStatusInProgress UserAssessmentStatus = "in_progress"
	AssessmentStatusCompleted  UserAssessmentStatus = "completed"
	AssessmentStatusCancelled  UserAssessmentStatus = "cancelled" // 用户主动取消或因某些条件中止
)

// UserAssessment 代表一个用户完成的评估
type UserAssessment struct {
	ID                uint                 `json:"id" gorm:"primaryKey"`
	UserID            string               `json:"user_id" gorm:"index"`
	Answers           []UserAnswer         `json:"answers" gorm:"type:jsonb"` // 推荐使用JSONB存储答案
	Status            UserAssessmentStatus `json:"status" gorm:"index"`
	CurrentQuestionID string               `json:"current_question_id,omitempty"`
	StartedAt         time.Time            `json:"started_at"`
	CompletedAt       *time.Time           `json:"completed_at,omitempty"` // 指针类型，允许为nil
	CreatedAt         time.Time            `json:"created_at"`
	UpdatedAt         time.Time            `json:"updated_at"`
}