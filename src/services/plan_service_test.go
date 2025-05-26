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

// MockAssessmentRepository (defined in assessment_service_test.go, re-declared here for plan service tests if needed,
// or ideally placed in a shared test mocks package)
// For this exercise, to avoid inter-file generation dependency, we can redefine a minimal version or use the one from assessment_service_test.go
// If these tests are run together (`go test ./src/services/...`), the mock from assessment_service_test.go might be accessible.
// However, for true unit testing, each _test.go file should be self-contained or use shared compiled mocks.
// Let's assume it's available or provide a minimal one here.

type MinimalMockAssessmentRepository struct {
	mock.Mock
}
func (m *MinimalMockAssessmentRepository) GetUserAssessmentByUserID(userID string, statusFilter ...models.UserAssessmentStatus) (*models.UserAssessment, error) {
	args := m.Called(userID, statusFilter) // Simplified, actual mock would be more specific
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.UserAssessment), args.Error(1)
}
// Add other methods if GeneratePlan actually uses them in more complex scenarios.
// For now, GeneratePlan is simplified and doesn't deeply use assessment results.
func (m *MinimalMockAssessmentRepository) CreateUserAssessment(assessment *models.UserAssessment) (*models.UserAssessment, error) {panic("implement me")}
func (m *MinimalMockAssessmentRepository) GetUserAssessmentByID(id uint) (*models.UserAssessment, error) {panic("implement me")}
func (m *MinimalMockAssessmentRepository) UpdateUserAssessment(assessment *models.UserAssessment) (*models.UserAssessment, error) {panic("implement me")}


func TestPlanService_GeneratePlan(t *testing.T) {
	mockPlanRepo := new(MockPlanRepository)
	mockAssessmentRepo := new(MinimalMockAssessmentRepository) // Use the minimal mock
	service := NewPlanService(mockPlanRepo, mockAssessmentRepo)
	userID := "userForPlan1"

	t.Run("Successfully generate a default plan", func(t *testing.T) {
		// Mock CreatePlan
		mockPlanRepo.On("CreatePlan", mock.MatchedBy(func(p *models.Plan) bool {
			return p.UserID == userID &&
				len(p.Tasks) == 3 && // Expecting 3 default tasks
				p.Status == models.PlanStatusActive
		})).Run(func(args mock.Arguments) {
			// Simulate GORM populating ID and Task IDs
			planArg := args.Get(0).(*models.Plan)
			planArg.ID = 1
			for i := range planArg.Tasks {
				planArg.Tasks[i].ID = uint(i + 1)
				planArg.Tasks[i].PlanID = planArg.ID
			}
		}).Return(nil).Once()
		
		// (Optional) Mock Assessment Repo if GeneratePlan uses it, for now it's simplified
		// mockAssessmentRepo.On("GetLatestCompletedAssessment", userID).Return(nil, nil).Once() // Example if used

		plan, err := service.GeneratePlan(userID)

		assert.NoError(t, err)
		assert.NotNil(t, plan)
		assert.Equal(t, userID, plan.UserID)
		assert.Equal(t, models.PlanStatusActive, plan.Status)
		assert.Len(t, plan.Tasks, 3)
		assert.Equal(t, "凯格尔运动", plan.Tasks[0].Title) // Check one of the default tasks
		assert.NotZero(t, plan.ID)
		assert.NotZero(t, plan.Tasks[0].ID)
		mockPlanRepo.AssertExpectations(t)
		// mockAssessmentRepo.AssertExpectations(t)
	})

	t.Run("Fail to generate plan if repository create fails", func(t *testing.T) {
		mockPlanRepo.On("CreatePlan", mock.AnythingOfType("*models.Plan")).Return(errors.New("DB error")).Once()

		plan, err := service.GeneratePlan(userID)

		assert.Error(t, err)
		assert.Nil(t, plan)
		assert.Contains(t, err.Error(), "failed to create plan")
		mockPlanRepo.AssertExpectations(t)
	})

	t.Run("Generate plan with empty userID", func(t *testing.T) {
		plan, err := service.GeneratePlan("")
		assert.Error(t, err)
		assert.Nil(t, plan)
		assert.EqualError(t, err, "userID cannot be empty")
		// No repo calls expected
		mockPlanRepo.AssertNotCalled(t, "CreatePlan", mock.Anything)
	})
}

func TestPlanService_MarkTaskCompleted(t *testing.T) {
	mockPlanRepo := new(MockPlanRepository)
	// Assessment repo not directly used by MarkTaskCompleted
	service := NewPlanService(mockPlanRepo, nil) 
	
	userID := "userTaskOwner"
	taskID := uint(1)
	planID := uint(10)

	t.Run("Successfully mark task as completed", func(t *testing.T) {
		task := &models.PlanTask{ID: taskID, PlanID: planID, Status: models.TaskStatusPending, UserID: userID /* conceptual, real check is via plan */}
		plan := &models.Plan{ID: planID, UserID: userID}

		mockPlanRepo.On("GetTaskByID", taskID).Return(task, nil).Once()
		mockPlanRepo.On("GetPlanByID", planID).Return(plan, nil).Once()
		mockPlanRepo.On("UpdatePlanTask", mock.MatchedBy(func(ut *models.PlanTask) bool {
			return ut.ID == taskID && ut.Status == models.TaskStatusCompleted && ut.IsCompleted == true && ut.CompletedAt.Valid == true
		})).Return(nil).Once()

		updatedTask, err := service.MarkTaskCompleted(taskID, userID)
		assert.NoError(t, err)
		assert.NotNil(t, updatedTask)
		assert.Equal(t, models.TaskStatusCompleted, updatedTask.Status)
		assert.True(t, updatedTask.IsCompleted)
		assert.True(t, updatedTask.CompletedAt.Valid)
		mockPlanRepo.AssertExpectations(t)
	})

	t.Run("Task not found", func(t *testing.T) {
		mockPlanRepo.On("GetTaskByID", taskID).Return(nil, nil).Once() // Task not found

		updatedTask, err := service.MarkTaskCompleted(taskID, userID)
		assert.Error(t, err)
		assert.Nil(t, updatedTask)
		assert.Contains(t, err.Error(), fmt.Sprintf("task with ID %d not found", taskID))
		mockPlanRepo.AssertExpectations(t)
	})

	t.Run("Unauthorized user", func(t *testing.T) {
		task := &models.PlanTask{ID: taskID, PlanID: planID, UserID: "anotherUser"}
		plan := &models.Plan{ID: planID, UserID: "anotherUser"} // Belongs to another user

		mockPlanRepo.On("GetTaskByID", taskID).Return(task, nil).Once()
		mockPlanRepo.On("GetPlanByID", planID).Return(plan, nil).Once()

		updatedTask, err := service.MarkTaskCompleted(taskID, userID) // userID is "userTaskOwner"
		assert.Error(t, err)
		assert.Nil(t, updatedTask)
		assert.Contains(t, err.Error(), "unauthorized to modify task")
		mockPlanRepo.AssertExpectations(t)
	})

	t.Run("Task already completed", func(t *testing.T) {
		now := time.Now()
		task := &models.PlanTask{
			ID: taskID, PlanID: planID, Status: models.TaskStatusCompleted, IsCompleted: true, CompletedAt: models.NullTime{Time: now, Valid: true},
		}
		plan := &models.Plan{ID: planID, UserID: userID}
		mockPlanRepo.On("GetTaskByID", taskID).Return(task, nil).Once()
		mockPlanRepo.On("GetPlanByID", planID).Return(plan, nil).Once()
		// No UpdatePlanTask should be called

		updatedTask, err := service.MarkTaskCompleted(taskID, userID)
		assert.NoError(t, err) // Service returns the task as is
		assert.NotNil(t, updatedTask)
		assert.Equal(t, models.TaskStatusCompleted, updatedTask.Status)
		mockPlanRepo.AssertExpectations(t)
	})
}


func TestPlanService_MarkTaskSkipped(t *testing.T) {
	mockPlanRepo := new(MockPlanRepository)
	service := NewPlanService(mockPlanRepo, nil) 
	
	userID := "userTaskOwnerSkip"
	taskID := uint(2)
	planID := uint(20)

	t.Run("Successfully mark task as skipped", func(t *testing.T) {
		task := &models.PlanTask{ID: taskID, PlanID: planID, Status: models.TaskStatusPending}
		plan := &models.Plan{ID: planID, UserID: userID}

		mockPlanRepo.On("GetTaskByID", taskID).Return(task, nil).Once()
		mockPlanRepo.On("GetPlanByID", planID).Return(plan, nil).Once()
		mockPlanRepo.On("UpdatePlanTask", mock.MatchedBy(func(ut *models.PlanTask) bool {
			return ut.ID == taskID && ut.Status == models.TaskStatusSkipped && ut.IsCompleted == false && ut.CompletedAt.Valid == false
		})).Return(nil).Once()

		updatedTask, err := service.MarkTaskSkipped(taskID, userID)
		assert.NoError(t, err)
		assert.NotNil(t, updatedTask)
		assert.Equal(t, models.TaskStatusSkipped, updatedTask.Status)
		assert.False(t, updatedTask.IsCompleted)
		assert.False(t, updatedTask.CompletedAt.Valid)
		mockPlanRepo.AssertExpectations(t)
	})

	t.Run("Cannot skip already completed task", func(t *testing.T) {
		now := time.Now()
		task := &models.PlanTask{
			ID: taskID, PlanID: planID, Status: models.TaskStatusCompleted, IsCompleted: true, CompletedAt: models.NullTime{Time: now, Valid: true},
		}
		plan := &models.Plan{ID: planID, UserID: userID}
		mockPlanRepo.On("GetTaskByID", taskID).Return(task, nil).Once()
		mockPlanRepo.On("GetPlanByID", planID).Return(plan, nil).Once()

		updatedTask, err := service.MarkTaskSkipped(taskID, userID)
		assert.Error(t, err)
		assert.Nil(t, updatedTask)
		assert.EqualError(t, err, "cannot skip an already completed task")
		mockPlanRepo.AssertExpectations(t)
	})
}

// MinimalMockAssessmentRepository is used here. If GeneratePlan becomes more complex
// and actually uses assessmentRepo's methods, the mock for it would need to be more complete
// or use the one from assessment_service_test.go (if structure allows sharing mocks).
// For now, GeneratePlan is simplified and doesn't use assessmentRepo, so a nil or minimal mock is fine.
// The PlanService constructor takes an AssessmentRepository, so we must provide one, even if it's not used by all methods.
// The `models.NullTime` was not part of the original models.PlanTask.CompletedAt but it is good practice.
// I've updated the mock assertion to check `Valid` field. If original model used `*time.Time`, then nil check.
// The provided model `plan.go` uses `gorm.NullTime`, so `Valid` is correct.

// Tests for GetPlanDetails and GetActivePlanForUser
func TestPlanService_GetPlan(t *testing.T) {
	mockPlanRepo := new(MockPlanRepository)
	service := NewPlanService(mockPlanRepo, nil)
	userID := "userGetPlan"
	planID := uint(1)

	t.Run("GetPlanDetails - success", func(t *testing.T) {
		expectedPlan := &models.Plan{ID: planID, UserID: userID, Title: "Test Plan", Tasks: []models.PlanTask{}}
		mockPlanRepo.On("GetPlanByID", planID).Return(expectedPlan, nil).Once()

		plan, err := service.GetPlanDetails(planID)
		assert.NoError(t, err)
		assert.Equal(t, expectedPlan, plan)
		mockPlanRepo.AssertExpectations(t)
	})

	t.Run("GetPlanDetails - not found", func(t *testing.T) {
		mockPlanRepo.On("GetPlanByID", planID).Return(nil, nil).Once() // Repo returns nil, nil for not found

		plan, err := service.GetPlanDetails(planID)
		assert.Error(t, err)
		assert.Nil(t, plan)
		assert.Contains(t, err.Error(), "not found")
		mockPlanRepo.AssertExpectations(t)
	})
	
	t.Run("GetActivePlanForUser - success", func(t *testing.T) {
		activePlan := &models.Plan{ID: planID, UserID: userID, Status: models.PlanStatusActive}
		otherPlan := &models.Plan{ID: planID + 1, UserID: userID, Status: models.PlanStatusCompleted}
		userPlans := []*models.Plan{otherPlan, activePlan} // Order might matter if repo doesn't sort by active status
		
		mockPlanRepo.On("GetPlansByUserID", userID).Return(userPlans, nil).Once()

		plan, err := service.GetActivePlanForUser(userID)
		assert.NoError(t, err)
		assert.Equal(t, activePlan, plan)
		mockPlanRepo.AssertExpectations(t)
	})

	t.Run("GetActivePlanForUser - no active plan found", func(t *testing.T) {
		completedPlan := &models.Plan{ID: planID, UserID: userID, Status: models.PlanStatusCompleted}
		userPlans := []*models.Plan{completedPlan}
		
		mockPlanRepo.On("GetPlansByUserID", userID).Return(userPlans, nil).Once()

		plan, err := service.GetActivePlanForUser(userID)
		assert.NoError(t, err) // Not finding an active plan is not an error itself
		assert.Nil(t, plan)
		mockPlanRepo.AssertExpectations(t)
	})
}
