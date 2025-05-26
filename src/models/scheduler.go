package models

import (
	"time"
)

// Task represents a scheduled task or job within the system.
// This model seems intended for a generic scheduler, which might not be fully implemented
// or utilized by the current "AI Bot Group Scheduler" (which is more about AI agent selection).
// If this is for a different type of scheduler (e.g., background jobs), it should be clarified.
type Task struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	Name        string    `json:"name"`        // Name of the task
	Description string    `json:"description"` // Description of what the task does
	CronExpr    string    `json:"cron_expr"`   // Cron expression for scheduling (e.g., "0 * * * *")
	Status      string    `json:"status"`      // Current status of the task (e.g., "pending", "running", "completed", "failed")
	CreatedAt   time.Time `json:"created_at"`  // Timestamp of task creation
	UpdatedAt   time.Time `json:"updated_at"`  // Timestamp of last task update
}

// TaskExecution records an instance of a task's execution.
// Similar to Task, its usage in the context of the current AI agent scheduler is unclear.
type TaskExecution struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	TaskID    uint      `json:"task_id"`     // Foreign key to the Task this execution belongs to
	StartTime time.Time `json:"start_time"`  // When the task execution started
	EndTime   time.Time `json:"end_time"`    // When the task execution finished
	Status    string    `json:"status"`      // Execution status (e.g., "success", "failed")
	Log       string    `json:"log"`         // Log output or error messages from the execution
}

// Potential_Future_Note: If these models (Task, TaskExecution) are not used by any current functionality,
// they might be remnants of a planned feature or a misunderstanding of the "scheduler" component's role.
// The existing `SchedulerService` in `src/services/scheduler_service.go` is focused on AI agent selection
// for chat responses, not on running cron-like background tasks.
// If these models are indeed for a different purpose, they should be documented as such,
// or removed if they are unused and not planned.
// For now, comments are translated assuming they are part of a generic task scheduling system.
