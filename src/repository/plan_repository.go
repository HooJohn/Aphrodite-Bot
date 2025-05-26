package repository

import (
	"errors"
	"log"
	"project/models"

	"gorm.io/gorm"
)

// PlanRepository defines the interface for interacting with plan and plan task data.
type PlanRepository interface {
	CreatePlan(plan *models.Plan) error
	GetPlanByID(planID uint) (*models.Plan, error)
	GetPlansByUserID(userID string) ([]*models.Plan, error)
	UpdatePlan(plan *models.Plan) error
	DeletePlan(planID uint, hardDelete bool) error // Added hardDelete flag for soft/hard delete choice
	CreatePlanTask(task *models.PlanTask) error
	GetPlanTasks(planID uint) ([]*models.PlanTask, error)
	GetTaskByID(taskID uint) (*models.PlanTask, error) // Added to support UpdatePlanTask and MarkTaskCompleted/Skipped
	UpdatePlanTask(task *models.PlanTask) error
	DeletePlanTask(taskID uint, hardDelete bool) error // Added hardDelete flag
}

type planRepository struct {
	db *gorm.DB
}

// NewPlanRepository creates a new instance of PlanRepository.
func NewPlanRepository(db *gorm.DB) PlanRepository {
	return &planRepository{db: db}
}

// CreatePlan creates a new plan in the database.
// It also saves associated tasks if they are included in the plan struct.
func (r *planRepository) CreatePlan(plan *models.Plan) error {
	if plan == nil {
		log.Printf("ERROR: [PlanRepository] CreatePlan: plan cannot be nil")
		return errors.New("plan cannot be nil")
	}
	err := r.db.Create(plan).Error
	if err != nil {
		log.Printf("ERROR: [PlanRepository] Failed to create plan for userID %s: %v", plan.UserID, err)
		return fmt.Errorf("failed to create plan for userID %s: %w", plan.UserID, err)
	}
	log.Printf("INFO: [PlanRepository] Successfully created plan ID %d for userID %s with %d tasks.", plan.ID, plan.UserID, len(plan.Tasks))
	return nil
}

// GetPlanByID retrieves a plan by its ID, preloading its tasks.
func (r *planRepository) GetPlanByID(planID uint) (*models.Plan, error) {
	var plan models.Plan
	log.Printf("INFO: [PlanRepository] Attempting to retrieve plan ID %d.", planID)
	err := r.db.Preload("Tasks").First(&plan, planID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("INFO: [PlanRepository] Plan with ID %d not found.", planID)
			return nil, nil // Not found
		}
		log.Printf("ERROR: [PlanRepository] Failed to retrieve plan ID %d: %v", planID, err)
		return nil, fmt.Errorf("failed to retrieve plan ID %d: %w", planID, err)
	}
	log.Printf("INFO: [PlanRepository] Successfully retrieved plan ID %d with %d tasks.", plan.ID, len(plan.Tasks))
	return &plan, nil
}

// GetPlansByUserID retrieves all plans for a given user ID, preloading their tasks.
func (r *planRepository) GetPlansByUserID(userID string) ([]*models.Plan, error) {
	var plans []*models.Plan
	log.Printf("INFO: [PlanRepository] Attempting to retrieve plans for userID %s.", userID)
	err := r.db.Preload("Tasks").Where("user_id = ?", userID).Order("created_at desc").Find(&plans).Error
	if err != nil {
		log.Printf("ERROR: [PlanRepository] Failed to retrieve plans for userID %s: %v", userID, err)
		return nil, fmt.Errorf("failed to retrieve plans for userID %s: %w", userID, err)
	}
	log.Printf("INFO: [PlanRepository] Successfully retrieved %d plans for userID %s.", len(plans), userID)
	return plans, nil // Returns empty slice if no plans found, which is fine
}

// UpdatePlan updates an existing plan in the database.
func (r *planRepository) UpdatePlan(plan *models.Plan) error {
	if plan == nil {
		log.Printf("ERROR: [PlanRepository] UpdatePlan: plan cannot be nil")
		return errors.New("plan cannot be nil")
	}
	if plan.ID == 0 {
		log.Printf("ERROR: [PlanRepository] UpdatePlan: plan ID must be provided for update")
		return errors.New("plan ID must be provided for update")
	}
	err := r.db.Save(plan).Error
	if err != nil {
		log.Printf("ERROR: [PlanRepository] Failed to update plan ID %d: %v", plan.ID, err)
		return fmt.Errorf("failed to update plan ID %d: %w", plan.ID, err)
	}
	log.Printf("INFO: [PlanRepository] Successfully updated plan ID %d.", plan.ID)
	return nil
}

// DeletePlan deletes a plan by its ID.
func (r *planRepository) DeletePlan(planID uint, hardDelete bool) error {
	var dbQuery *gorm.DB
	action := "soft-deleted"
	if hardDelete {
		dbQuery = r.db.Unscoped()
		action = "hard-deleted (permanently)"
	} else {
		dbQuery = r.db
	}
	log.Printf("INFO: [PlanRepository] Attempting to %s plan ID %d.", action, planID)

	// GORM's default behavior with "constraint:OnDelete:CASCADE" on Plan.Tasks
	// should handle deletion of associated tasks if the database supports and is configured for it
	// during hard delete. For soft delete, tasks are typically not automatically soft-deleted by CASCADE.
	// This may require manual soft deletion of tasks or specific GORM hooks if that's the desired behavior.
	// Current implementation relies on GORM's behavior for the Plan entity itself.

	err := dbQuery.Delete(&models.Plan{}, planID).Error
	if err != nil {
		log.Printf("ERROR: [PlanRepository] Failed to %s plan ID %d: %v", action, planID, err)
		return fmt.Errorf("failed to %s plan ID %d: %w", action, planID, err)
	}
	log.Printf("INFO: [PlanRepository] Successfully %s plan ID %d.", action, planID)
	return nil
}

// CreatePlanTask creates a new task associated with a plan.
func (r *planRepository) CreatePlanTask(task *models.PlanTask) error {
	if task == nil {
		log.Printf("ERROR: [PlanRepository] CreatePlanTask: task cannot be nil")
		return errors.New("task cannot be nil")
	}
	if task.PlanID == 0 {
		log.Printf("ERROR: [PlanRepository] CreatePlanTask: task must be associated with a PlanID (PlanID is 0)")
		return errors.New("task must be associated with a PlanID")
	}
	err := r.db.Create(task).Error
	if err != nil {
		log.Printf("ERROR: [PlanRepository] Failed to create task '%s' for plan ID %d: %v", task.Title, task.PlanID, err)
		return fmt.Errorf("failed to create task '%s' for plan ID %d: %w", task.Title, task.PlanID, err)
	}
	log.Printf("INFO: [PlanRepository] Successfully created task ID %d ('%s') for plan ID %d.", task.ID, task.Title, task.PlanID)
	return nil
}

// GetPlanTasks retrieves all tasks for a given plan ID.
func (r *planRepository) GetPlanTasks(planID uint) ([]*models.PlanTask, error) {
	var tasks []*models.PlanTask
	log.Printf("INFO: [PlanRepository] Attempting to retrieve tasks for plan ID %d.", planID)
	err := r.db.Where("plan_id = ?", planID).Order("`order` asc, created_at asc").Find(&tasks).Error
	if err != nil {
		log.Printf("ERROR: [PlanRepository] Failed to retrieve tasks for plan ID %d: %v", planID, err)
		return nil, fmt.Errorf("failed to retrieve tasks for plan ID %d: %w", planID, err)
	}
	log.Printf("INFO: [PlanRepository] Successfully retrieved %d tasks for plan ID %d.", len(tasks), planID)
	return tasks, nil
}

// GetTaskByID retrieves a single task by its ID.
func (r *planRepository) GetTaskByID(taskID uint) (*models.PlanTask, error) {
	var task models.PlanTask
	log.Printf("INFO: [PlanRepository] Attempting to retrieve task ID %d.", taskID)
	err := r.db.First(&task, taskID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("INFO: [PlanRepository] Task with ID %d not found.", taskID)
			return nil, nil // Not found
		}
		log.Printf("ERROR: [PlanRepository] Failed to retrieve task ID %d: %v", taskID, err)
		return nil, fmt.Errorf("failed to retrieve task ID %d: %w", taskID, err)
	}
	log.Printf("INFO: [PlanRepository] Successfully retrieved task ID %d.", task.ID)
	return &task, nil
}

// UpdatePlanTask updates an existing plan task.
func (r *planRepository) UpdatePlanTask(task *models.PlanTask) error {
	if task == nil {
		log.Printf("ERROR: [PlanRepository] UpdatePlanTask: task cannot be nil")
		return errors.New("task cannot be nil")
	}
	if task.ID == 0 {
		log.Printf("ERROR: [PlanRepository] UpdatePlanTask: task ID must be provided for update")
		return errors.New("task ID must be provided for update")
	}
	err := r.db.Save(task).Error
	if err != nil {
		log.Printf("ERROR: [PlanRepository] Failed to update task ID %d ('%s'): %v", task.ID, task.Title, err)
		return fmt.Errorf("failed to update task ID %d: %w", task.ID, err)
	}
	log.Printf("INFO: [PlanRepository] Successfully updated task ID %d ('%s').", task.ID, task.Title)
	return nil
}

// DeletePlanTask deletes a task by its ID.
func (r *planRepository) DeletePlanTask(taskID uint, hardDelete bool) error {
	var dbQuery *gorm.DB
	action := "soft-deleted"
	if hardDelete {
		dbQuery = r.db.Unscoped()
		action = "hard-deleted (permanently)"
	} else {
		dbQuery = r.db
	}
	log.Printf("INFO: [PlanRepository] Attempting to %s task ID %d.", action, taskID)
	err := dbQuery.Delete(&models.PlanTask{}, taskID).Error
	if err != nil {
		log.Printf("ERROR: [PlanRepository] Failed to %s task ID %d: %v", action, taskID, err)
		return fmt.Errorf("failed to %s task ID %d: %w", action, taskID, err)
	}
	log.Printf("INFO: [PlanRepository] Successfully %s task ID %d.", action, taskID)
	return nil
}
