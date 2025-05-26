package models

import (
	"time"
)

// AssessmentQuestionType defines the type of an assessment question.
type AssessmentQuestionType string

const (
	QuestionTypeSingleChoice AssessmentQuestionType = "single_choice" // Radio buttons
	QuestionTypeMultiChoice  AssessmentQuestionType = "multi_choice"  // Checkboxes
	QuestionTypeOpenText     AssessmentQuestionType = "open_text"     // Free text input
	QuestionTypeConfirmation AssessmentQuestionType = "confirmation"  // Yes/No or Agree/Disagree
)

// AssessmentQuestion defines a question in the assessment questionnaire.
// Note: These question definitions are currently hardcoded in AssessmentService,
// so GORM tags are not needed here for this specific struct if it's not stored in DB directly.
type AssessmentQuestion struct {
	ID             string                 `json:"id"`           // Unique identifier for the question
	Order          int                    `json:"order"`          // Display order of the question
	Text           string                 `json:"text"`           // The question text
	QuestionType   AssessmentQuestionType `json:"question_type"`  // Type of the question (e.g., single_choice)
	Options        []string               `json:"options,omitempty"` // Available options for choice-based questions
	IsRequired     bool                   `json:"is_required"`    // Whether the question must be answered
	NextQuestionID string                 `json:"next_question_id,omitempty"` // Used for simple fixed flow, or as a fallback in branching logic
	// Category       string                 `json:"category,omitempty"`    // Optional: Category of the question (e.g., "Lifestyle", "Health History")
	// HelperText     string                 `json:"helper_text,omitempty"` // Optional: Additional text to help user understand the question
}

// UserAnswer represents a user's answer to a specific assessment question.
type UserAnswer struct {
	QuestionID string   `json:"question_id"` // ID of the question being answered
	Answer     []string `json:"answer"`      // User's answer(s). Slice to support multi-choice and single/text.
	// AnsweredAt time.Time `json:"answered_at"` // Optional: Timestamp when the answer was provided
}

// UserAssessmentStatus defines the status of a user's assessment process.
type UserAssessmentStatus string

const (
	AssessmentStatusInProgress UserAssessmentStatus = "in_progress" // Assessment is currently being filled out
	AssessmentStatusCompleted  UserAssessmentStatus = "completed"  // Assessment has been fully completed
	AssessmentStatusCancelled  UserAssessmentStatus = "cancelled"  // Assessment was cancelled by the user or system
)

// UserAssessment represents a user's assessment session, including their answers and status.
type UserAssessment struct {
	ID                uint                 `json:"id" gorm:"primaryKey"`
	UserID            string               `json:"user_id" gorm:"index"`      // Identifier of the user taking the assessment
	Answers           []UserAnswer         `json:"answers" gorm:"type:jsonb"` // User's answers, recommended to store as JSONB in DB
	Status            UserAssessmentStatus `json:"status" gorm:"index"`       // Current status of the assessment (e.g., in_progress, completed)
	CurrentQuestionID string               `json:"current_question_id,omitempty"` // ID of the current question the user is on
	StartedAt         time.Time            `json:"started_at"`                // Timestamp when the assessment was started
	CompletedAt       *time.Time           `json:"completed_at,omitempty"`    // Timestamp when the assessment was completed (pointer, allows nil)
	CreatedAt         time.Time            `json:"created_at"`                // Timestamp of record creation
	UpdatedAt         time.Time            `json:"updated_at"`                // Timestamp of last record update
}