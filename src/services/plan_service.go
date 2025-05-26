package services

import (
	"errors"
	"fmt"
	"log"
	"project/models"
	"project/repository"
	"time"
)

// PlanService defines the interface for managing plans and tasks.
type PlanService interface {
	GeneratePlan(userID string) (*models.Plan, error)
	GetPlanDetails(planID uint) (*models.Plan, error)
	GetActivePlanForUser(userID string) (*models.Plan, error) // Returns the first active plan found
	MarkTaskCompleted(taskID uint, userID string) (*models.PlanTask, error) // Added userID for authorization
	MarkTaskSkipped(taskID uint, userID string) (*models.PlanTask, error)   // Added userID for authorization
}

type planService struct {
	planRepo       repository.PlanRepository
	assessmentRepo repository.AssessmentRepository // To fetch assessment results
}

// NewPlanService creates a new instance of PlanService.
func NewPlanService(planRepo repository.PlanRepository, assessmentRepo repository.AssessmentRepository) PlanService {
	return &planService{
		planRepo:       planRepo,
		assessmentRepo: assessmentRepo,
	}
}

// GeneratePlan creates a new plan for a user based on simplified logic or assessment data.
func (s *planService) GeneratePlan(userID string) (*models.Plan, error) {
	if userID == "" {
		log.Printf("WARN: [PlanService] GeneratePlan called with empty userID.")
		return nil, errors.New("userID cannot be empty")
	}
	log.Printf("INFO: [PlanService] Attempting to generate plan for userID: %s", userID)

	// Placeholder: Fetch user's completed assessment.
	// For now, we'll simulate this. Assume assessmentRepo has a method like:
	// assessment, err := s.assessmentRepo.GetLatestCompletedAssessment(userID)
	// if err != nil { log.Printf("WARN: [PlanService] Error fetching assessment for plan generation for userID %s: %v", userID, err) }
	// if assessment == nil { log.Printf("INFO: [PlanService] No completed assessment found for userID %s for plan generation.", userID) }

	// Simplified plan generation logic:
	planTitle := "我的健康提升计划"
	planDescription := "一个旨在改善整体健康的初步计划。"
	tasks := []models.PlanTask{}

	// Example rule: If assessment indicates interest in "凯格尔运动" (Kegel exercises)
	// This is a placeholder. Real logic would parse `assessment.Results` or `assessment.Goals`.
	// For now, let's add a default set of tasks.
	tasks = append(tasks, models.PlanTask{
		Type:        models.TaskTypeExercise,
		Title:       "凯格尔运动",
		Description: "每日进行凯格尔运动，强化盆底肌。",
		Frequency:   "每日2次",
		Duration:    "每次5分钟",
		Status:      models.TaskStatusPending,
		Order:       1,
	})
	tasks = append(tasks, models.PlanTask{
		Type:        models.TaskTypeHabit,
		Title:       "健康饮水",
		Description: "确保每日饮用足够的水（约8杯）。",
		Frequency:   "每日",
		Duration:    "全天",
		Status:      models.TaskStatusPending,
		Order:       2,
	})
	tasks = append(tasks, models.PlanTask{
		Type:        models.TaskTypeKnowledge,
		Title:       "了解健康饮食",
		Description: "阅读一篇关于均衡饮食的文章。",
		Frequency:   "本周一次",
		Duration:    "约15分钟阅读",
		Status:      models.TaskStatusPending,
		Order:       3,
	})

	newPlan := &models.Plan{
		UserID:      userID,
		Title:       planTitle,
		Description: planDescription,
		Status:      models.PlanStatusActive, // Start as active
		Tasks:       tasks,                  // GORM will create these associated tasks
	}

	err := s.planRepo.CreatePlan(newPlan)
	if err != nil {
		errMsg := fmt.Sprintf("failed to create plan for userID %s", userID)
		log.Printf("ERROR: [PlanService] %s: %v", errMsg, err)
		return nil, fmt.Errorf("%s: %w", errMsg, err)
	}

	log.Printf("INFO: [PlanService] Successfully generated plan ID %d for userID %s with %d tasks.", newPlan.ID, newPlan.UserID, len(newPlan.Tasks))
	return newPlan, nil
}

// GetPlanDetails retrieves a plan and its tasks.
func (s *planService) GetPlanDetails(planID uint) (*models.Plan, error) {
	log.Printf("INFO: [PlanService] Attempting to get details for planID: %d", planID)
	plan, err := s.planRepo.GetPlanByID(planID)
	if err != nil {
		errMsg := fmt.Sprintf("failed to get plan details for planID %d from repository", planID)
		log.Printf("ERROR: [PlanService] %s: %v", errMsg, err)
		return nil, fmt.Errorf("%s: %w", errMsg, err)
	}
	if plan == nil { // Repository returns (nil, nil) for not found
		log.Printf("WARN: [PlanService] Plan with ID %d not found.", planID)
		return nil, fmt.Errorf("plan with ID %d not found", planID) // Return specific error
	}
	log.Printf("INFO: [PlanService] Successfully retrieved plan details for planID %d.", planID)
	return plan, nil
}

// GetActivePlanForUser retrieves the first active plan for a user.
func (s *planService) GetActivePlanForUser(userID string) (*models.Plan, error) {
	log.Printf("INFO: [PlanService] Attempting to get active plan for userID: %s", userID)
	plans, err := s.planRepo.GetPlansByUserID(userID)
	if err != nil {
		errMsg := fmt.Sprintf("failed to get plans for userID %s from repository", userID)
		log.Printf("ERROR: [PlanService] %s: %v", errMsg, err)
		return nil, fmt.Errorf("%s: %w", errMsg, err)
	}

	for _, plan := range plans {
		if plan.Status == models.PlanStatusActive {
			log.Printf("INFO: [PlanService] Found active plan ID %d for userID %s.", plan.ID, userID)
			return plan, nil
		}
	}
	log.Printf("INFO: [PlanService] No active plan found for userID %s.", userID)
	return nil, nil // No active plan found is not an error in itself
}

// MarkTaskCompleted marks a task as completed.
func (s *planService) MarkTaskCompleted(taskID uint, userID string) (*models.PlanTask, error) {
	log.Printf("INFO: [PlanService] UserID '%s' attempting to mark taskID %d as completed.", userID, taskID)
	task, err := s.planRepo.GetTaskByID(taskID)
	if err != nil {
		errMsg := fmt.Sprintf("failed to fetch task ID %d for completion", taskID)
		log.Printf("ERROR: [PlanService] %s: %v", errMsg, err)
		return nil, fmt.Errorf("%s: %w", errMsg, err)
	}
	if task == nil {
		log.Printf("WARN: [PlanService] Task with ID %d not found for completion attempt by userID '%s'.", taskID, userID)
		return nil, fmt.Errorf("task with ID %d not found", taskID)
	}

	plan, err := s.planRepo.GetPlanByID(task.PlanID)
	if err != nil {
		errMsg := fmt.Sprintf("failed to fetch plan ID %d for task ID %d (userID '%s')", task.PlanID, taskID, userID)
		log.Printf("ERROR: [PlanService] %s: %v", errMsg, err)
		return nil, fmt.Errorf("%s: %w", errMsg, err)
	}
	if plan == nil { // Should not happen if task exists and has valid PlanID, but good check
		log.Printf("ERROR: [PlanService] Plan not found for task ID %d (PlanID: %d), userID '%s'. Data integrity issue?", taskID, task.PlanID, userID)
		return nil, fmt.Errorf("plan associated with task %d not found", taskID)
	}

	if plan.UserID != userID {
		log.Printf("WARN: [PlanService] Unauthorized attempt by userID '%s' to complete taskID %d (belongs to userID '%s').", userID, taskID, plan.UserID)
		return nil, fmt.Errorf("unauthorized to modify task %d", taskID) // Specific error for authorization
	}

	if task.Status == models.TaskStatusCompleted {
		log.Printf("INFO: [PlanService] TaskID %d is already completed. No action taken for userID '%s'.", taskID, userID)
		return task, nil
	}

	task.Status = models.TaskStatusCompleted
	task.IsCompleted = true
	now := time.Now()
	task.CompletedAt.Time = now
	task.CompletedAt.Valid = true


	err = s.planRepo.UpdatePlanTask(task)
	if err != nil {
		errMsg := fmt.Sprintf("failed to update task ID %d to completed for userID '%s'", taskID, userID)
		log.Printf("ERROR: [PlanService] %s: %v", errMsg, err)
		return nil, fmt.Errorf("%s: %w", errMsg, err)
	}

	log.Printf("INFO: [PlanService] TaskID %d marked as completed for userID '%s'.", taskID, userID)
	return task, nil
}

// MarkTaskSkipped marks a task as skipped.
func (s *planService) MarkTaskSkipped(taskID uint, userID string) (*models.PlanTask, error) {
	log.Printf("INFO: [PlanService] UserID '%s' attempting to mark taskID %d as skipped.", userID, taskID)
	task, err := s.planRepo.GetTaskByID(taskID)
	if err != nil {
		errMsg := fmt.Sprintf("failed to fetch task ID %d for skipping", taskID)
		log.Printf("ERROR: [PlanService] %s: %v", errMsg, err)
		return nil, fmt.Errorf("%s: %w", errMsg, err)
	}
	if task == nil {
		log.Printf("WARN: [PlanService] Task with ID %d not found for skip attempt by userID '%s'.", taskID, userID)
		return nil, fmt.Errorf("task with ID %d not found", taskID)
	}

	plan, err := s.planRepo.GetPlanByID(task.PlanID)
	if err != nil {
		errMsg := fmt.Sprintf("failed to fetch plan ID %d for task ID %d (userID '%s')", task.PlanID, taskID, userID)
		log.Printf("ERROR: [PlanService] %s: %v", errMsg, err)
		return nil, fmt.Errorf("%s: %w", errMsg, err)
	}
	if plan == nil {
		log.Printf("ERROR: [PlanService] Plan not found for task ID %d (PlanID: %d), userID '%s'. Data integrity issue?", taskID, task.PlanID, userID)
		return nil, fmt.Errorf("plan associated with task %d not found", taskID)
	}

	if plan.UserID != userID {
		log.Printf("WARN: [PlanService] Unauthorized attempt by userID '%s' to skip taskID %d (belongs to userID '%s').", userID, taskID, plan.UserID)
		return nil, fmt.Errorf("unauthorized to modify task %d", taskID)
	}

	if task.Status == models.TaskStatusSkipped {
		log.Printf("INFO: [PlanService] TaskID %d is already skipped. No action taken for userID '%s'.", taskID, userID)
		return task, nil
	}
	if task.Status == models.TaskStatusCompleted {
		log.Printf("WARN: [PlanService] UserID '%s' attempted to skip already completed taskID %d.", userID, taskID)
		return nil, errors.New("cannot skip an already completed task")
	}

	task.Status = models.TaskStatusSkipped
	task.IsCompleted = false 
	task.CompletedAt.Valid = false 

	err = s.planRepo.UpdatePlanTask(task)
	if err != nil {
		errMsg := fmt.Sprintf("failed to update task ID %d to skipped for userID '%s'", taskID, userID)
		log.Printf("ERROR: [PlanService] %s: %v", errMsg, err)
		return nil, fmt.Errorf("%s: %w", errMsg, err)
	}
	log.Printf("INFO: [PlanService] TaskID %d marked as skipped for userID '%s'.", taskID, userID)
	return task, nil
}
