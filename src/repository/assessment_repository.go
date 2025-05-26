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
		return nil, errors.New("用户ID不能为空")
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
	r.userIndex[assessment.UserID] = append(r.userIndex[assessment.UserID], assessment.ID) // 更新用户索引

	log.Printf("创建用户评估成功: ID=%d, UserID=%s, Status=%s", assessment.ID, assessment.UserID, assessment.Status)
	return assessment, nil
}

// GetUserAssessmentByID 根据ID获取用户评估记录
func (r *assessmentRepository) GetUserAssessmentByID(id uint) (*models.UserAssessment, error) {
	r.muAssessments.RLock()
	defer r.muAssessments.RUnlock()

	assessment, exists := r.userAssessments[id]
	if !exists {
		return nil, fmt.Errorf("未找到ID为 %d 的用户评估记录", id)
	}
	return assessment, nil
}

// GetUserAssessmentByUserID 获取指定用户最新的或特定状态的评估记录
func (r *assessmentRepository) GetUserAssessmentByUserID(userID string, statusFilter ...models.UserAssessmentStatus) (*models.UserAssessment, error) {
	r.muAssessments.RLock()
	defer r.muAssessments.RUnlock()

	assessmentIDs, userExists := r.userIndex[userID]
	if !userExists || len(assessmentIDs) == 0 {
		return nil, fmt.Errorf("用户 '%s' 没有任何评估记录", userID)
	}

	var latestMatchingAssessment *models.UserAssessment
	var latestTimestamp time.Time

	targetStatus := "" // 如果不提供statusFilter，则不按status过滤，只找最新的
	if len(statusFilter) > 0 && statusFilter[0] != "" {
		targetStatus = string(statusFilter[0]) // 将 models.UserAssessmentStatus 转为 string
	}

	for i := len(assessmentIDs) - 1; i >= 0; i-- { // 从后向前遍历，更有可能先找到最新的
		assessmentID := assessmentIDs[i]
		assessment, exists := r.userAssessments[assessmentID]
		if !exists { // 理论上不应该发生，索引和主数据应一致
			log.Printf("警告: 用户索引中的评估ID %d 在主数据中未找到 (UserID: %s)", assessmentID, userID)
			continue
		}

		if targetStatus != "" { // 如果指定了状态过滤
			if string(assessment.Status) == targetStatus { // 状态匹配
				// 如果是查找特定状态，通常我们期望的是该状态下最新的一个（按UpdatedAt或CreatedAt）
				// 或者业务上只允许一个特定状态的评估存在
				if latestMatchingAssessment == nil || assessment.UpdatedAt.After(latestTimestamp) {
					latestMatchingAssessment = assessment
					latestTimestamp = assessment.UpdatedAt
				}
			}
		} else { // 如果没有指定状态过滤，则找该用户最新的评估（无论状态）
			if latestMatchingAssessment == nil || assessment.UpdatedAt.After(latestTimestamp) {
				latestMatchingAssessment = assessment
				latestTimestamp = assessment.UpdatedAt
			}
		}
	}

	if latestMatchingAssessment == nil {
		if targetStatus != "" {
			return nil, fmt.Errorf("未找到用户 '%s' 状态为 '%s' 的评估记录", userID, targetStatus)
		}
		return nil, fmt.Errorf("用户 '%s' 没有符合条件的评估记录", userID) // 理论上如果userIndex有记录，这里应该能找到至少一个
	}
	
	log.Printf("为用户 '%s' 找到评估记录 ID: %d, Status: %s (Filter: '%s')", userID, latestMatchingAssessment.ID, latestMatchingAssessment.Status, targetStatus)
	return latestMatchingAssessment, nil
}

// UpdateUserAssessment 更新用户评估记录
func (r *assessmentRepository) UpdateUserAssessment(assessment *models.UserAssessment) (*models.UserAssessment, error) {
	r.muAssessments.Lock()
	defer r.muAssessments.Unlock()

	_, exists := r.userAssessments[assessment.ID]
	if !exists {
		return nil, fmt.Errorf("更新失败：未找到ID为 %d 的用户评估记录", assessment.ID)
	}

	// UserID 和 StartedAt, CreatedAt 通常不应被外部更新
	originalAssessment := r.userAssessments[assessment.ID]
	assessment.UserID = originalAssessment.UserID
	assessment.StartedAt = originalAssessment.StartedAt
	assessment.CreatedAt = originalAssessment.CreatedAt
	assessment.UpdatedAt = time.Now() // 确保更新时间被设置

	r.userAssessments[assessment.ID] = assessment // 直接用传入的 assessment 替换（已处理不可变字段）
	log.Printf("更新用户评估成功: ID=%d, UserID=%s, Status=%s", assessment.ID, assessment.UserID, assessment.Status)
	return assessment, nil
}