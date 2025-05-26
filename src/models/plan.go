package models

import (
	"time"

	"gorm.io/gorm"
)

// PlanStatus defines the possible statuses for a plan.
type PlanStatus string

const (
	PlanStatusActive    PlanStatus = "active"
	PlanStatusCompleted PlanStatus = "completed"
	PlanStatusCancelled PlanStatus = "cancelled"
	PlanStatusPending   PlanStatus = "pending" // Pending generation or user confirmation
)

// Plan represents a user's personalized health or habit plan.
type Plan struct {
	ID          uint           `gorm:"primarykey"`
	UserID      string         `gorm:"index;not null"` // To link to the user
	Title       string         `gorm:"not null"`
	Description string         `gorm:"type:text"`
	Status      PlanStatus     `gorm:"type:varchar(50);default:'pending';not null"`
	CreatedAt   time.Time      `gorm:"autoCreateTime"`
	UpdatedAt   time.Time      `gorm:"autoUpdateTime"`
	DeletedAt   gorm.DeletedAt `gorm:"index"` // For soft deletes
	Tasks       []PlanTask     `gorm:"foreignKey:PlanID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"` // Has many relationship
}

// TableName specifies the table name for the Plan model.
func (Plan) TableName() string {
	return "plans"
}

// TaskType defines the category of a plan task.
type TaskType string

const (
	TaskTypeExercise  TaskType = "exercise"
	TaskTypeHabit     TaskType = "habit"
	TaskTypeKnowledge TaskType = "knowledge"
	TaskTypeGeneric   TaskType = "generic"
)

// TaskStatus defines the possible statuses for a plan task.
type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusSkipped   TaskStatus = "skipped"
	TaskStatusFailed    TaskStatus = "failed" // Could be used if a task wasn't met
)

// PlanTask represents an individual task within a Plan.
type PlanTask struct {
	ID          uint           `gorm:"primarykey"`
	PlanID      uint           `gorm:"index;not null"` // Foreign key to Plan
	Type        TaskType       `gorm:"type:varchar(50);not null"`
	Title       string         `gorm:"not null"`
	Description string         `gorm:"type:text"`
	Frequency   string         // e.g., "daily", "3 times a week", "once on 2024-03-15"
	Duration    string         // e.g., "15 minutes", "1 chapter", "3 sets of 10 reps"
	Status      TaskStatus     `gorm:"type:varchar(50);default:'pending';not null"`
	IsCompleted bool           `gorm:"default:false"`                           // Derived from Status == TaskStatusCompleted
	CompletedAt gorm.NullTime  // time.Time can be nullable using gorm.NullTime or *time.Time
	Order       int            `gorm:"default:0"` // For ordering tasks within a plan
	CreatedAt   time.Time      `gorm:"autoCreateTime"`
	UpdatedAt   time.Time      `gorm:"autoUpdateTime"`
	DeletedAt   gorm.DeletedAt `gorm:"index"` // For soft deletes
}

// TableName specifies the table name for the PlanTask model.
func (PlanTask) TableName() string {
	return "plan_tasks"
}
