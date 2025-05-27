package services

import (
	"errors"
	"fmt"
	"log"
	"project/models"
	"project/repository"
	"strings"
	"time"
)

const (
	KeyActivityKegel = "凯格尔运动" // Example key activity
	KeyActivityWater = "健康饮水" // Another example
	daysInWeek       = 7
	daysInMonth      = 30 // Approximate for simplicity
	dateFormat       = "2006-01-02"
)

// ProgressService defines the interface for generating progress reports.
type ProgressService interface {
	GenerateProgressReport(userID string, periodType string, referenceDateStr string) (*models.ProgressReportResponse, error)
}

type progressService struct {
	planRepo repository.PlanRepository
}

// NewProgressService creates a new instance of ProgressService.
func NewProgressService(planRepo repository.PlanRepository) ProgressService {
	return &progressService{
		planRepo: planRepo,
	}
}

// GenerateProgressReport generates a progress report for a given user and period.
func (s *progressService) GenerateProgressReport(userID string, periodType string, referenceDateStr string) (*models.ProgressReportResponse, error) {
	if userID == "" {
		return nil, errors.New("userID cannot be empty")
	}
	if s.planRepo == nil {
		log.Printf("ERROR: [ProgressService] PlanRepository is not initialized for userID: %s", userID)
		return nil, errors.New("internal server error: plan repository not available")
	}

	// 1. Determine Date Range
	endDate := time.Now()
	if referenceDateStr != "" {
		parsedDate, err := time.Parse(dateFormat, referenceDateStr)
		if err != nil {
			log.Printf("WARN: [ProgressService] Invalid referenceDateStr '%s' for userID %s: %v. Defaulting to now.", referenceDateStr, userID, err)
		} else {
			endDate = parsedDate
		}
	}
	// Set to end of the day for endDate to include all activities on that day
	endDate = time.Date(endDate.Year(), endDate.Month(), endDate.Day(), 23, 59, 59, 0, endDate.Location())

	var startDate time.Time
	switch periodType {
	case "last_7_days":
		startDate = endDate.AddDate(0, 0, -6) // endDate is the 7th day
	case "last_30_days":
		startDate = endDate.AddDate(0, 0, -29) // endDate is the 30th day
	default:
		log.Printf("WARN: [ProgressService] Unsupported periodType '%s' for userID %s. Defaulting to last_7_days.", periodType, userID)
		periodType = "last_7_days" // Default to last_7_days
		startDate = endDate.AddDate(0, 0, -6)
	}
	// Set to start of the day for startDate
	startDate = time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, startDate.Location())
	
	reportPeriod := models.ReportPeriod{
		StartDate: startDate.Format(dateFormat),
		EndDate:   endDate.Format(dateFormat),
		PeriodType: periodType,
	}
	log.Printf("INFO: [ProgressService] Generating report for userID %s, period: %s to %s (%s)", userID, reportPeriod.StartDate, reportPeriod.EndDate, periodType)

	// 2. Fetch Data
	allUserPlans, err := s.planRepo.GetPlansByUserID(userID)
	if err != nil {
		log.Printf("ERROR: [ProgressService] Failed to get plans for userID %s: %v", userID, err)
		return nil, fmt.Errorf("failed to retrieve plans: %w", err)
	}

	var relevantPlans []*models.Plan
	var tasksForOverallSummary []models.PlanTask
	tasksCompletedByType := make(map[models.TaskType]int)

	overallSummary := models.OverallSummary{TasksCompletedByType: tasksCompletedByType}
	
	keyActivitiesToTrack := map[string]*models.ActivityProgress{
		KeyActivityKegel: {ActivityTitle: KeyActivityKegel},
		KeyActivityWater: {ActivityTitle: KeyActivityWater},
		// Add more key activities here if needed
	}

	for _, plan := range allUserPlans {
		// Plan relevance: active or completed within the period, or started before period end and active/pending.
		planIsRelevant := false
		if plan.Status == models.PlanStatusActive || plan.Status == models.PlanStatusPending {
			// Plan is currently active/pending: relevant if it started before the report period ends.
			if !plan.CreatedAt.After(endDate) {
				planIsRelevant = true
			}
		} else if plan.Status == models.PlanStatusCompleted {
			// Plan is completed: relevant if it completed within the report period OR was active during any part of it.
			if plan.CompletedAt.Valid && !plan.CompletedAt.Time.Before(startDate) && !plan.CompletedAt.Time.After(endDate) {
				planIsRelevant = true // Completed within period
			} else if plan.CompletedAt.Valid && plan.CompletedAt.Time.After(endDate) && !plan.CreatedAt.After(endDate) {
				planIsRelevant = true // Was active during some part of the period, completed after
			} else if !plan.CreatedAt.After(endDate) && (plan.CompletedAt.Valid == false || plan.CompletedAt.Time.After(startDate)) {
                 // Started before period end, and was active at least at the start of period
                 planIsRelevant = true
            }
		}
		// Add more sophisticated overlap logic if needed for plans cancelled during period etc.

		if planIsRelevant {
			relevantPlans = append(relevantPlans, plan)
			// Note: plan.Tasks are preloaded by planRepo.GetPlansByUserID
			for _, task := range plan.Tasks {
				tasksForOverallSummary = append(tasksForOverallSummary, task) // Count all tasks in relevant plans

				// Metrics for OverallSummary
				if task.Status == models.TaskStatusCompleted && task.CompletedAt.Valid &&
					!task.CompletedAt.Time.Before(startDate) && !task.CompletedAt.Time.After(endDate) {
					overallSummary.CompletedTasks++
					tasksCompletedByType[task.Type]++
				}
				if task.Status == models.TaskStatusSkipped { // Assuming skipped tasks are counted if skipped within period (needs timestamp if so)
					// For simplicity, count all skipped tasks in relevant plans. Refine if skip date needed.
					// Let's assume for now a task is "skipped" if its status is skipped and it falls within the period.
					// This requires tasks to have a relevant date (e.g. due date or update date).
					// For MVP, we might just count tasks with status "skipped" from active plans.
					// If task.UpdatedAt is within period and status is skipped:
					if task.UpdatedAt.After(startDate) && task.UpdatedAt.Before(endDate) {
						 overallSummary.SkippedTasks++
					}
				}
				
				// Metrics for ActivityProgress
				if progress, ok := keyActivitiesToTrack[task.Title]; ok {
					// Simplified ScheduledCount: if task exists in an active plan during period, assume it was scheduled daily.
					// This needs to be refined based on actual task frequency and plan active days.
					// For MVP, let's assume a task is "scheduled" once if its plan is active.
					// A better MVP for "daily" tasks: count days plan was active in period.
					
					// Count as scheduled if the plan containing this task was active at any point during the report period
					// This is a major simplification.
					if plan.Status == models.PlanStatusActive { // Could also check plan.CreatedAt/plan.CompletedAt against period
						progress.ScheduledCount++ // Simplistic: counts each instance of task title in relevant plans.
					}

					if task.Status == models.TaskStatusCompleted && task.CompletedAt.Valid &&
						!task.CompletedAt.Time.Before(startDate) && !task.CompletedAt.Time.After(endDate) {
						progress.CompletedCount++
						if progress.LastCompletedDate == nil {
							dateStr := task.CompletedAt.Time.Format(dateFormat)
							progress.LastCompletedDate = &dateStr
						} else {
							currentLast, _ := time.Parse(dateFormat, *progress.LastCompletedDate)
							if task.CompletedAt.Time.After(currentLast) {
								dateStr := task.CompletedAt.Time.Format(dateFormat)
								progress.LastCompletedDate = &dateStr
							}
						}
					}
				}
			}
		}
	}

	overallSummary.TotalPlansConsidered = len(relevantPlans)
	overallSummary.TotalTasks = len(tasksForOverallSummary) // This is a raw count of all tasks in relevant plans.
	
	// Refined TotalTasks for CompletionRate: tasks that could have been completed in period.
	// This is complex. For MVP, let's use a simpler definition or acknowledge the limitation.
	// For now, TotalTasks - SkippedTasks.
	denominator := float64(overallSummary.TotalTasks - overallSummary.SkippedTasks)
	if denominator > 0 {
		overallSummary.CompletionRate = float64(overallSummary.CompletedTasks) / denominator
	} else if overallSummary.CompletedTasks > 0 { // Completed tasks but 0 denominator (e.g. all tasks skipped or total tasks is 0)
		overallSummary.CompletionRate = 1.0 // Or undefined, based on product decision
	} else {
		overallSummary.CompletionRate = 0.0
	}


	var activityProgressSlice []models.ActivityProgress
	for _, activity := range keyActivitiesToTrack {
		if activity.ScheduledCount > 0 {
			activity.AdherenceRate = float64(activity.CompletedCount) / float64(activity.ScheduledCount)
		} else if activity.CompletedCount > 0 { // Completed but not "scheduled" by this simple logic
			activity.AdherenceRate = 1.0 
		}
		activityProgressSlice = append(activityProgressSlice, *activity)
	}
	
	response := &models.ProgressReportResponse{
		UserID:           userID,
		ReportPeriod:     reportPeriod,
		OverallSummary:   overallSummary,
		ActivityProgress: activityProgressSlice,
		GeneratedAt:      time.Now().UTC(),
	}

	log.Printf("INFO: [ProgressService] Successfully generated progress report for userID %s for period %s to %s.", userID, reportPeriod.StartDate, reportPeriod.EndDate)
	return response, nil
}

// Helper function to estimate scheduled count for a daily task within a period
// This is a more accurate helper if we decide to use it.
func estimateScheduledDaily(plan models.Plan, task models.PlanTask, periodStart, periodEnd time.Time) int {
	if !strings.ToLower(task.Frequency) == "daily" { // Adapt for other recognized daily patterns
		return 0 // Or some other logic for non-daily tasks
	}

	// Determine the intersection of the plan's active duration and the report period
	effectiveStart := periodStart
	if plan.CreatedAt.After(periodStart) {
		effectiveStart = plan.CreatedAt
	}

	effectiveEnd := periodEnd
	if plan.CompletedAt.Valid && plan.CompletedAt.Time.Before(periodEnd) {
		effectiveEnd = plan.CompletedAt.Time
	}
	if plan.Status == models.PlanStatusCancelled && plan.UpdatedAt.Before(periodEnd) { // Assuming UpdatedAt is when it was cancelled
	    if plan.UpdatedAt.Before(effectiveEnd) {
	        effectiveEnd = plan.UpdatedAt
	    }
	}
	
	if effectiveEnd.Before(effectiveStart) {
		return 0
	}

	// Calculate number of days in the effective period for the task
	// Add 1 because it's inclusive of start and end day
	return int(effectiveEnd.Sub(effectiveStart).Hours()/24) + 1
}
