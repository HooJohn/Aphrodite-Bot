package repository

import (
	"errors"
	"fmt"
	"log"
	"project/models"
	"sync"
	"time"
)

// AssessmentRepository 评估仓库接口 (保持不变)
type AssessmentRepository interface {
	CreateUserAssessment(assessment *models.UserAssessment) (*models.UserAssessment, error)
	GetUserAssessmentByID(id uint) (*models.UserAssessment, error)
	GetUserAssessmentByUserID(userID string, statusFilter ...models.UserAssessmentStatus) (*models.UserAssessment, error)
	UpdateUserAssessment(assessment *models.UserAssessment) (*models.UserAssessment, error)
}

// assessmentRepository 评估仓库实现 (内存版)
type assessmentRepository struct {
	userAssessments  map[uint]*models.UserAssessment // 按评估ID存储
	userIndex        map[string][]uint             // 按UserID索引评估ID列表，方便快速查找某用户的所有评估
	nextAssessmentID uint
	muAssessments    sync.RWMutex
}

// NewAssessmentRepository 创建评估仓库实例
func NewAssessmentRepository() AssessmentRepository {
	return &assessmentRepository{
		userAssessments:  make(map[uint]*models.UserAssessment),
		userIndex:        make(map[string][]uint),
		nextAssessmentID: 1,
	}
}

// CreateUserAssessment 创建一个新的用户评估记录
func (r *assessmentRepository) CreateUserAssessment(assessment *models.UserAssessment) (*models.UserAssessment, error) {
	r.muAssessments.Lock()
	defer r.muAssessments.Unlock()

	if assessment.UserID == "" {
		log.Printf("ERROR: [AssessmentRepository] CreateUserAssessment: UserID cannot be empty.")
		return nil, errors.New("user ID cannot be empty")
	}

	assessment.ID = r.nextAssessmentID
	r.nextAssessmentID++
	assessment.CreatedAt = time.Now()
	assessment.UpdatedAt = time.Now()

	if assessment.Status == "" {
		assessment.Status = models.AssessmentStatusInProgress
	}
	if assessment.StartedAt.IsZero() {
		assessment.StartedAt = time.Now()
	}

	r.userAssessments[assessment.ID] = assessment
	r.userIndex[assessment.UserID] = append(r.userIndex[assessment.UserID], assessment.ID)

	log.Printf("INFO: [AssessmentRepository] Created user assessment: ID=%d, UserID=%s, Status=%s", assessment.ID, assessment.UserID, assessment.Status)
	return assessment, nil
}

// GetUserAssessmentByID 根据ID获取用户评估记录
func (r *assessmentRepository) GetUserAssessmentByID(id uint) (*models.UserAssessment, error) {
	r.muAssessments.RLock()
	defer r.muAssessments.RUnlock()

	assessment, exists := r.userAssessments[id]
	if !exists {
		log.Printf("WARN: [AssessmentRepository] GetUserAssessmentByID: Assessment with ID %d not found.", id)
		return nil, fmt.Errorf("assessment record with ID %d not found", id)
	}
	log.Printf("INFO: [AssessmentRepository] Retrieved assessment ID %d.", id)
	return assessment, nil
}

// GetUserAssessmentByUserID 获取指定用户最新的或特定状态的评估记录
func (r *assessmentRepository) GetUserAssessmentByUserID(userID string, statusFilter ...models.UserAssessmentStatus) (*models.UserAssessment, error) {
	r.muAssessments.RLock()
	defer r.muAssessments.RUnlock()

	assessmentIDs, userExists := r.userIndex[userID]
	if !userExists || len(assessmentIDs) == 0 {
		log.Printf("INFO: [AssessmentRepository] GetUserAssessmentByUserID: No assessment records found for userID '%s'.", userID)
		return nil, nil // Changed to return nil, nil for not found, service layer will interpret
	}

	var latestMatchingAssessment *models.UserAssessment
	var latestTimestamp time.Time

	targetStatus := "" // 如果不提供statusFilter，则不按status过滤，只找最新的
	if len(statusFilter) > 0 && statusFilter[0] != "" {
		targetStatus = string(statusFilter[0]) // 将 models.UserAssessmentStatus 转为 string
	}

	for i := len(assessmentIDs) - 1; i >= 0; i-- { 
		assessmentID := assessmentIDs[i]
		assessment, exists := r.userAssessments[assessmentID]
		if !exists {
			log.Printf("ERROR: [AssessmentRepository] Data inconsistency: AssessmentID %d found in userIndex for userID '%s' but not in main userAssessments map.", assessmentID, userID)
			continue 
		}

		if targetStatus != "" { 
			if string(assessment.Status) == targetStatus { 
				if latestMatchingAssessment == nil || assessment.UpdatedAt.After(latestTimestamp) {
					latestMatchingAssessment = assessment
					latestTimestamp = assessment.UpdatedAt
				}
			}
		} else { 
			if latestMatchingAssessment == nil || assessment.UpdatedAt.After(latestTimestamp) {
				latestMatchingAssessment = assessment
				latestTimestamp = assessment.UpdatedAt
			}
		}
	}

	if latestMatchingAssessment == nil {
		log.Printf("INFO: [AssessmentRepository] GetUserAssessmentByUserID: No assessment found for userID '%s' with status filter '%s'.", userID, targetStatus)
		return nil, nil // Changed to return nil, nil for not found
	}
	
	log.Printf("INFO: [AssessmentRepository] Retrieved assessment ID %d (Status: %s) for userID '%s' (Filter: '%s').", latestMatchingAssessment.ID, latestMatchingAssessment.Status, userID, targetStatus)
	return latestMatchingAssessment, nil
}

// UpdateUserAssessment 更新用户评估记录
func (r *assessmentRepository) UpdateUserAssessment(assessment *models.UserAssessment) (*models.UserAssessment, error) {
	r.muAssessments.Lock()
	defer r.muAssessments.Unlock()

	originalAssessment, exists := r.userAssessments[assessment.ID]
	if !exists {
		log.Printf("WARN: [AssessmentRepository] UpdateUserAssessment: Assessment with ID %d not found.", assessment.ID)
		return nil, fmt.Errorf("update failed: assessment record with ID %d not found", assessment.ID)
	}

	// Preserve immutable fields
	assessment.UserID = originalAssessment.UserID
	assessment.StartedAt = originalAssessment.StartedAt
	assessment.CreatedAt = originalAssessment.CreatedAt
	assessment.UpdatedAt = time.Now() // Ensure update time is set

	r.userAssessments[assessment.ID] = assessment
	log.Printf("INFO: [AssessmentRepository] Updated user assessment: ID=%d, UserID=%s, Status=%s", assessment.ID, assessment.UserID, assessment.Status)
	return assessment, nil
}