package models

import "time"

// ReportPeriod defines the time range for the progress report.
type ReportPeriod struct {
	StartDate string `json:"start_date"` // YYYY-MM-DD
	EndDate   string `json:"end_date"`   // YYYY-MM-DD
	PeriodType string `json:"period_type"` // e.g., "last_7_days", "last_30_days", "custom_range"
}

// OverallSummary provides a high-level summary of progress.
type OverallSummary struct {
	TotalPlansConsidered int                `json:"total_plans_considered"` // Number of plans active or completed in period
	TotalTasks           int                `json:"total_tasks"`            // Total tasks from these plans (potentially filtered by period relevance)
	CompletedTasks       int                `json:"completed_tasks"`        // Tasks completed within the period
	SkippedTasks         int                `json:"skipped_tasks"`          // Tasks skipped within the period
	CompletionRate       float64            `json:"completion_rate"`        // (CompletedTasks / (TotalTasks - SkippedTasks))
	TasksCompletedByType map[TaskType]int   `json:"tasks_completed_by_type"` // e.g., {"exercise": 10, "habit": 5}
}

// ActivityProgress tracks progress for a specific recurring activity/task.
type ActivityProgress struct {
	ActivityTitle     string    `json:"activity_title"`      // e.g., "凯格尔运动"
	CompletedCount    int       `json:"completed_count"`     // How many times completed in period
	ScheduledCount    int       `json:"scheduled_count"`     // How many times it was expected to be done (simplified for MVP)
	AdherenceRate     float64   `json:"adherence_rate"`      // CompletedCount / ScheduledCount
	LastCompletedDate *string   `json:"last_completed_date,omitempty"` // YYYY-MM-DD, nil if not completed in period
}

// ProgressReportResponse is the main structure for the progress report API.
type ProgressReportResponse struct {
	UserID         string             `json:"user_id"`
	ReportPeriod   ReportPeriod       `json:"report_period"`
	OverallSummary OverallSummary     `json:"overall_summary"`
	ActivityProgress []ActivityProgress `json:"activity_progress,omitempty"` // Progress for specific key activities
	GeneratedAt    time.Time          `json:"generated_at"`
	// Future fields: Streaks, Achievements, Comparison to previous period, etc.
}
