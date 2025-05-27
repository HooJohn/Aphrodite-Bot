package services

import (
	"errors"
	"fmt"
	"project/models"
	"project/repository"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockPlanRepository is a mock type for the PlanRepository interface
type MockPlanRepository struct {
	mock.Mock
}

func (m *MockPlanRepository) CreatePlan(plan *models.Plan) error {
	args := m.Called(plan)
	return args.Error(0)
}

func (m *MockPlanRepository) GetPlanByID(planID uint) (*models.Plan, error) {
	args := m.Called(planID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Plan), args.Error(1)
}

func (m *MockPlanRepository) GetPlansByUserID(userID string) ([]*models.Plan, error) {
	args := m.Called(userID)
	// Handle nil case for the first argument if the test expects (nil, error)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.Plan), args.Error(1)
}

func (m *MockPlanRepository) UpdatePlan(plan *models.Plan) error {
	args := m.Called(plan)
	return args.Error(0)
}

func (m *MockPlanRepository) DeletePlan(planID uint, hardDelete bool) error {
	args := m.Called(planID, hardDelete)
	return args.Error(0)
}

func (m *MockPlanRepository) CreatePlanTask(task *models.PlanTask) error {
	args := m.Called(task)
	return args.Error(0)
}

func (m *MockPlanRepository) GetPlanTasks(planID uint) ([]*models.PlanTask, error) {
	args := m.Called(planID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.PlanTask), args.Error(1)
}

func (m *MockPlanRepository) GetTaskByID(taskID uint) (*models.PlanTask, error) {
	args := m.Called(taskID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.PlanTask), args.Error(1)
}

func (m *MockPlanRepository) UpdatePlanTask(task *models.PlanTask) error {
	args := m.Called(task)
	return args.Error(0)
}

func (m *MockPlanRepository) DeletePlanTask(taskID uint, hardDelete bool) error {
	args := m.Called(taskID, hardDelete)
	return args.Error(0)
}

// --- Test Helper Functions ---
func newTestPlan(id uint, userID string, status models.PlanStatus, createdAt time.Time, tasks []models.PlanTask) *models.Plan {
	plan := &models.Plan{
		ID:        id,
		UserID:    userID,
		Title:     fmt.Sprintf("Test Plan %d", id),
		Status:    status,
		CreatedAt: createdAt,
		Tasks:     []models.PlanTask{}, // Tasks will be attached if provided
	}
	if status == models.PlanStatusCompleted {
		completedAt := createdAt.AddDate(0,0,5) // Assume completed 5 days after creation for testing
		plan.CompletedAt = models.NullTime{Time: completedAt, Valid: true}
	}
	for _, task := range tasks {
		task.PlanID = id // Ensure task is linked to this plan
		plan.Tasks = append(plan.Tasks, task)
	}
	return plan
}

func newTestTask(id uint, title string, taskType models.TaskType, status models.TaskStatus, completedAt *time.Time) models.PlanTask {
	task := models.PlanTask{
		ID:          id,
		Title:       title,
		Type:        taskType,
		Status:      status,
		IsCompleted: status == models.TaskStatusCompleted,
	}
	if completedAt != nil && status == models.TaskStatusCompleted {
		task.CompletedAt = models.NullTime{Time: *completedAt, Valid: true}
	}
	return task
}


// --- Tests for GenerateProgressReport ---
func TestProgressService_GenerateProgressReport(t *testing.T) {
	mockPlanRepo := new(MockPlanRepository)
	progressSvc := NewProgressService(mockPlanRepo)
	userID := "testUser"
	refDateToday := time.Now()
	
	// Helper to parse dates for test data consistency
	parseDate := func(dateStr string) time.Time {
		d, _ := time.Parse(dateFormat, dateStr)
		return d
	}

	t.Run("Scenario 1: Basic Happy Path (last_7_days)", func(t *testing.T) {
		// Setup: Tasks completed within the last 7 days
		task1Time := refDateToday.AddDate(0, 0, -2) // 2 days ago
		task2Time := refDateToday.AddDate(0, 0, -5) // 5 days ago
		
		plan1Tasks := []models.PlanTask{
			newTestTask(1, KeyActivityKegel, models.TaskTypeExercise, models.TaskStatusCompleted, &task1Time),
			newTestTask(2, "Morning Jog", models.TaskTypeExercise, models.TaskStatusPending, nil),
			newTestTask(3, KeyActivityWater, models.TaskTypeHabit, models.TaskStatusCompleted, &task2Time),
		}
		plan1 := newTestPlan(1, userID, models.PlanStatusActive, refDateToday.AddDate(0,0,-10), plan1Tasks)
		
		mockPlanRepo.On("GetPlansByUserID", userID).Return([]*models.Plan{plan1}, nil).Once()
		// Note: Progress service currently iterates plan.Tasks, doesn't call GetPlanTasks separately.

		report, err := progressSvc.GenerateProgressReport(userID, "last_7_days", refDateToday.Format(dateFormat))

		assert.NoError(t, err)
		assert.NotNil(t, report)
		assert.Equal(t, userID, report.UserID)
		assert.Equal(t, 1, report.OverallSummary.TotalPlansConsidered)
		assert.Equal(t, 3, report.OverallSummary.TotalTasks) // All tasks in the relevant plan
		assert.Equal(t, 2, report.OverallSummary.CompletedTasks) // task1 and task3
		assert.Equal(t, 0, report.OverallSummary.SkippedTasks)
		assert.InDelta(t, float64(2)/float64(3), report.OverallSummary.CompletionRate, 0.001)
		assert.Equal(t, 1, report.OverallSummary.TasksCompletedByType[models.TaskTypeExercise])
		assert.Equal(t, 1, report.OverallSummary.TasksCompletedByType[models.TaskTypeHabit])

		foundKegel := false
		for _, ap := range report.ActivityProgress {
			if ap.ActivityTitle == KeyActivityKegel {
				foundKegel = true
				assert.Equal(t, 1, ap.CompletedCount)
				// Simplified ScheduledCount in service counts each task instance in active plans
				assert.Equal(t, 1, ap.ScheduledCount) 
				assert.InDelta(t, 1.0, ap.AdherenceRate, 0.001)
				assert.NotNil(t, ap.LastCompletedDate)
				assert.Equal(t, task1Time.Format(dateFormat), *ap.LastCompletedDate)
			}
		}
		assert.True(t, foundKegel, "Kegel activity progress not found")
		mockPlanRepo.AssertExpectations(t)
	})

	t.Run("Scenario 2: No Plans Found for User", func(t *testing.T) {
		mockPlanRepo.On("GetPlansByUserID", userID).Return([]*models.Plan{}, nil).Once()

		report, err := progressSvc.GenerateProgressReport(userID, "last_7_days", "")
		assert.NoError(t, err)
		assert.NotNil(t, report)
		assert.Equal(t, 0, report.OverallSummary.TotalPlansConsidered)
		assert.Equal(t, 0, report.OverallSummary.TotalTasks)
		assert.Equal(t, 0, report.OverallSummary.CompletedTasks)
		assert.Equal(t, 0.0, report.OverallSummary.CompletionRate)
		assert.Empty(t, report.ActivityProgress)
		mockPlanRepo.AssertExpectations(t)
	})
	
	t.Run("Scenario 3: No Tasks in Relevant Plans", func(t *testing.T) {
		plan1 := newTestPlan(1, userID, models.PlanStatusActive, refDateToday.AddDate(0,0,-10), []models.PlanTask{})
		mockPlanRepo.On("GetPlansByUserID", userID).Return([]*models.Plan{plan1}, nil).Once()

		report, err := progressSvc.GenerateProgressReport(userID, "last_7_days", "")
		assert.NoError(t, err)
		assert.NotNil(t, report)
		assert.Equal(t, 1, report.OverallSummary.TotalPlansConsidered)
		assert.Equal(t, 0, report.OverallSummary.TotalTasks)
		assert.Equal(t, 0, report.OverallSummary.CompletedTasks)
		assert.Equal(t, 0.0, report.OverallSummary.CompletionRate)
		mockPlanRepo.AssertExpectations(t)
	})

	t.Run("Scenario 4: All Tasks Completed", func(t *testing.T) {
		task1Time := refDateToday.AddDate(0,0,-1)
		task2Time := refDateToday.AddDate(0,0,-2)
		planTasks := []models.PlanTask{
			newTestTask(1, KeyActivityKegel, models.TaskTypeExercise, models.TaskStatusCompleted, &task1Time),
			newTestTask(2, KeyActivityWater, models.TaskTypeHabit, models.TaskStatusCompleted, &task2Time),
		}
		plan1 := newTestPlan(1, userID, models.PlanStatusActive, refDateToday.AddDate(0,0,-5), planTasks)
		mockPlanRepo.On("GetPlansByUserID", userID).Return([]*models.Plan{plan1}, nil).Once()

		report, err := progressSvc.GenerateProgressReport(userID, "last_7_days", "")
		assert.NoError(t, err)
		assert.NotNil(t, report)
		assert.Equal(t, 2, report.OverallSummary.CompletedTasks)
		assert.Equal(t, 2, report.OverallSummary.TotalTasks)
		assert.Equal(t, 1.0, report.OverallSummary.CompletionRate)
		mockPlanRepo.AssertExpectations(t)
	})

	t.Run("Scenario 5: Date Range Logic (last_30_days with referenceDate)", func(t *testing.T) {
		referenceDate := parseDate("2024-03-15")
		startDateExpected := "2024-02-15" // Approx, service calculates -29 days from end of ref date
		
		// Task completed within this 30-day window
		taskCompletedInPeriod := referenceDate.AddDate(0,0,-10)
		// Task completed outside (before) this 30-day window
		taskCompletedBeforePeriod := referenceDate.AddDate(0,0,-35)

		planTasks := []models.PlanTask{
			newTestTask(1, "Task In Period", models.TaskTypeGeneric, models.TaskStatusCompleted, &taskCompletedInPeriod),
			newTestTask(2, "Task Before Period", models.TaskTypeGeneric, models.TaskStatusCompleted, &taskCompletedBeforePeriod),
		}
		plan1 := newTestPlan(1, userID, models.PlanStatusActive, referenceDate.AddDate(0,0,-40), planTasks)
		mockPlanRepo.On("GetPlansByUserID", userID).Return([]*models.Plan{plan1}, nil).Once()
		
		report, err := progressSvc.GenerateProgressReport(userID, "last_30_days", "2024-03-15")
		assert.NoError(t, err)
		assert.NotNil(t, report)
		assert.Equal(t, "2024-03-15", report.ReportPeriod.EndDate)
		// Service calculates start date as refDate - 29 days, then start of that day.
		// Feb 15, 2024 is correct for ref March 15, 2024 (-29 days)
		assert.True(t, strings.HasPrefix(report.ReportPeriod.StartDate, "2024-02-1"), "Start date should be around Feb 15th")
		// A more precise check would involve calculating the exact start date like the service does.
		// For this test, let's check if it's roughly correct.
		// For "2024-03-15", service calculates startDate as 2024-02-15 (after -29 days, then start of day)
		assert.Equal(t, startDateExpected, report.ReportPeriod.StartDate)


		assert.Equal(t, 1, report.OverallSummary.CompletedTasks) // Only "Task In Period"
		assert.Equal(t, 2, report.OverallSummary.TotalTasks)
		mockPlanRepo.AssertExpectations(t)
	})

	t.Run("Scenario 6: Error from Repository", func(t *testing.T) {
		mockPlanRepo.On("GetPlansByUserID", userID).Return(nil, errors.New("database connection error")).Once()
		
		report, err := progressSvc.GenerateProgressReport(userID, "last_7_days", "")
		assert.Error(t, err)
		assert.Nil(t, report)
		assert.Contains(t, err.Error(), "failed to retrieve plans")
		mockPlanRepo.AssertExpectations(t)
	})

	t.Run("Scenario 7: ActivityProgress - Key Activity Not Found in tasks", func(t *testing.T) {
		nonKeyTaskTime := refDateToday.AddDate(0,0,-3)
		planTasks := []models.PlanTask{
			newTestTask(1, "Some Other Exercise", models.TaskTypeExercise, models.TaskStatusCompleted, &nonKeyTaskTime),
		}
		plan1 := newTestPlan(1, userID, models.PlanStatusActive, refDateToday.AddDate(0,0,-5), planTasks)
		mockPlanRepo.On("GetPlansByUserID", userID).Return([]*models.Plan{plan1}, nil).Once()

		report, err := progressSvc.GenerateProgressReport(userID, "last_7_days", "")
		assert.NoError(t, err)
		assert.NotNil(t, report)
		for _, ap := range report.ActivityProgress {
			if ap.ActivityTitle == KeyActivityKegel {
				assert.Equal(t, 0, ap.CompletedCount, "Kegel completed count should be 0")
				// ScheduledCount might be 0 or >0 depending on how it's calculated for non-existent tasks
				// Current simplified logic might not add it to keyActivitiesToTrack map if no task has this title.
				// The service initializes keyActivitiesToTrack, so it will be present.
				assert.Equal(t, 0, ap.ScheduledCount, "Kegel scheduled count should be 0 if no task title matches")
			}
		}
		mockPlanRepo.AssertExpectations(t)
	})
	
	t.Run("Scenario 8: Division by Zero for Rates", func(t *testing.T) {
		// Case 1: No tasks at all (TotalTasks = 0, SkippedTasks = 0)
		plan1 := newTestPlan(1, userID, models.PlanStatusActive, refDateToday.AddDate(0,0,-5), []models.PlanTask{})
		mockPlanRepo.On("GetPlansByUserID", userID).Return([]*models.Plan{plan1}, nil).Once()

		report, err := progressSvc.GenerateProgressReport(userID, "last_7_days", "")
		assert.NoError(t, err)
		assert.NotNil(t, report)
		assert.Equal(t, 0.0, report.OverallSummary.CompletionRate, "CompletionRate should be 0.0 if no tasks")
		
		// Case 2: All tasks skipped (TotalTasks > 0, but TotalTasks - SkippedTasks = 0)
		skippedTaskTime := refDateToday.AddDate(0,0,-1) // Assuming UpdatedAt for skipped task is within period
		planTasksSkipped := []models.PlanTask{
			{ID: 1, Title: KeyActivityKegel, Type: models.TaskTypeExercise, Status: models.TaskStatusSkipped, UpdatedAt: skippedTaskTime},
		}
		plan2 := newTestPlan(2, userID, models.PlanStatusActive, refDateToday.AddDate(0,0,-5), planTasksSkipped)
		// Reset mock for new On call
		mockPlanRepo.ExpectedCalls = nil 
		mockPlanRepo.On("GetPlansByUserID", userID).Return([]*models.Plan{plan2}, nil).Once()

		report2, err2 := progressSvc.GenerateProgressReport(userID, "last_7_days", "")
		assert.NoError(t, err2)
		assert.NotNil(t, report2)
		assert.Equal(t, 0, report2.OverallSummary.CompletedTasks)
		assert.Equal(t, 1, report2.OverallSummary.SkippedTasks) // Assuming service counts it
		assert.Equal(t, 1, report2.OverallSummary.TotalTasks)
		// Denominator is TotalTasks - SkippedTasks. If 1-1=0, rate should be 0 or 1 if completed > 0. Here completed is 0.
		assert.Equal(t, 0.0, report2.OverallSummary.CompletionRate, "CompletionRate should be 0.0 if all tasks skipped")

		for _, ap := range report2.ActivityProgress {
			if ap.ActivityTitle == KeyActivityKegel {
				assert.Equal(t, 0.0, ap.AdherenceRate, "AdherenceRate should be 0.0 if scheduled but not completed")
			}
		}
		mockPlanRepo.AssertExpectations(t) // This will assert for the last On call
	})
}
