package services

import (
	"errors"
	"fmt"
	"log"
	"project/models"
	"project/repository"
	"sort"
	"time"
	"strings"
)

// AssessmentService 评估服务接口 (保持不变)
type AssessmentService interface {
	StartOrContinueAssessment(userID string) (question *models.AssessmentQuestion, assessment *models.UserAssessment, userVisibleError error)
	SubmitAnswer(userID string, questionID string, answerValues []string) (question *models.AssessmentQuestion, assessment *models.UserAssessment, userVisibleError error)
	GetAssessmentResult(userID string) (*models.UserAssessment, error)
}

// assessmentService 评估服务实现
type assessmentService struct {
	repo          repository.AssessmentRepository
	questions     []models.AssessmentQuestion          // 有序问题列表
	questionsByID map[string]*models.AssessmentQuestion // 按ID快速查找问题指针
}

// NewAssessmentService 创建评估服务实例
func NewAssessmentService(repo repository.AssessmentRepository) AssessmentService {
	definedQuestions := getDefaultAssessmentQuestions() // 从辅助函数获取问题定义

	sort.Slice(definedQuestions, func(i, j int) bool { // 确保问题按Order排序
		return definedQuestions[i].Order < definedQuestions[j].Order
	})

	questionsMap := make(map[string]*models.AssessmentQuestion)
	tempQuestions := make([]models.AssessmentQuestion, len(definedQuestions)) // 创建副本以存储指针
	for i, q := range definedQuestions {
		// 为 questionsByID 存储指针，这样 s.questions 中的修改会同步（虽然这里是只读）
		// 或者，直接让 s.questions 存储指针类型 []*models.AssessmentQuestion
		// 为了简单，这里我们让 s.questions 存储值类型，s.questionsByID 存储指针
		// 但更好的做法可能是让 s.questions 也是 []*models.AssessmentQuestion
		// 这里我们先复制一份到 tempQuestions，然后取地址
		tempQuestions[i] = q 
		questionsMap[q.ID] = &tempQuestions[i]
	}
	
	return &assessmentService{
		repo:          repo,
		questions:     definedQuestions, // 存储值类型的问题列表
		questionsByID: questionsMap,     // 存储指向临时副本中元素的指针
	}
}

// getDefaultAssessmentQuestions (保持不变，内容已在之前提供)
func getDefaultAssessmentQuestions() []models.AssessmentQuestion {
    // ... (返回我们定义的问题列表)
	return []models.AssessmentQuestion{
		{ID: "q_welcome", Order: 0, Text: "您好！为了给您提供更个性化的服务，我们需要了解一些您的基本情况。整个过程大约需要5-10分钟，您的数据将被严格保密。您准备好开始了吗？", QuestionType: models.QuestionTypeConfirmation, Options: []string{"是的，我准备好了", "不了，下次吧"}, IsRequired:   true},
		{ID: "q_age_group", Order: 1, Text: "您的年龄段是？", QuestionType: models.QuestionTypeSingleChoice, Options: []string{"18-24岁", "25-34岁", "35-44岁", "45-54岁", "55岁及以上"}, IsRequired: true},
		{ID: "q_chronic_diseases", Order: 2, Text: "您是否有医生诊断过的慢性疾病（如高血压、糖尿病、心脏病等）？如果有，请简要说明，如无可填写“无”。", QuestionType: models.QuestionTypeOpenText, IsRequired: true},
		{ID: "q_medications", Order: 3, Text: "您目前是否正在长期服用任何处方药？如果是，请简要说明药品名称或类型，如无可填写“无”。", QuestionType: models.QuestionTypeOpenText, IsRequired: true},
		{ID: "q_exercise_habits", Order: 4, Text: "您目前的运动频率如何？", QuestionType: models.QuestionTypeSingleChoice, Options: []string{"几乎不运动", "每周1-2次", "每周3-4次", "每周5次及以上"}, IsRequired: true},
		{ID: "q_sleep_quality", Order: 5, Text: "您通常的睡眠质量如何？", QuestionType: models.QuestionTypeSingleChoice, Options: []string{"很好，能保证7-8小时且精力充沛", "一般，偶尔睡眠不足或质量不高", "较差，经常失眠或睡眠质量差"}, IsRequired: true},
		{ID: "q_stress_level", Order: 6, Text: "您感觉自己目前的整体压力水平如何？（1为极低，5为极高）", QuestionType: models.QuestionTypeSingleChoice, Options: []string{"1 (极低)", "2 (较低)", "3 (中等)", "4 (较高)", "5 (极高)"}, IsRequired: true},
		{ID: "q_sex_frequency_satisfaction", Order: 7, Text: "您对目前的性生活频率满意吗？", QuestionType: models.QuestionTypeSingleChoice, Options: []string{"非常满意", "比较满意", "一般", "不太满意", "非常不满意", "目前没有性生活"}, IsRequired: true},
		{ID: "q_sex_ability_concerns", Order: 8, Text: "在性能力方面（如勃起、持久度、性欲等），您目前主要有哪些困惑或担忧？（多选，如无困惑可选择“无明显困惑”）", QuestionType: models.QuestionTypeMultiChoice, Options: []string{"勃起困难或维持时间短", "早泄或持久度不足", "性欲低下", "高潮困难", "性交疼痛", "对自身表现焦虑", "缺乏性知识或技巧", "无明显困惑"}, IsRequired: true},
		{ID: "q_main_goals", Order: 9, Text: "通过我们的AI伙伴，您最希望达成的核心目标是什么？（多选）", QuestionType: models.QuestionTypeMultiChoice, Options: []string{"获取科学的性健康知识", "提升性能力（如持久度、硬度）", "改善与伴侣的性沟通", "调整不健康的性习惯", "缓解性焦虑或压力", "制定个性化的锻炼计划", "了解并尝试健康的性行为模式"}, IsRequired: true},
		{ID: "q_privacy_consent", Order: 10, Text: "我们再次强调，您的所有回答都将被严格保密，仅用于为您提供个性化的健康建议和计划。您是否同意我们处理您的这些信息？", QuestionType: models.QuestionTypeConfirmation, Options: []string{"我同意并继续", "我不同意"}, IsRequired: true},
	}
}


// StartOrContinueAssessment (逻辑已优化)
func (s *assessmentService) StartOrContinueAssessment(userID string) (*models.AssessmentQuestion, *models.UserAssessment, error) {
	assessment, err := s.repo.GetUserAssessmentByUserID(userID, models.AssessmentStatusInProgress)
	if err != nil {
		// 检查是否是“未找到进行中评估”的特定错误类型，如果是，则创建新的
		// 这里依赖 repository 返回一个可识别的错误，或者更简单地，如果没有找到就返回 nil, nil
		// 假设 GetUserAssessmentByUserID 在找不到时返回 (nil, nil) 或 (nil, specificError)
		if assessment == nil { // 如果找不到进行中的评估 (或者 repo 返回 nil, nil)
			log.Printf("未找到用户 '%s' 的进行中评估，将创建新评估。", userID)
			newAssessment := &models.UserAssessment{
				UserID:    userID,
				Status:    models.AssessmentStatusInProgress,
				StartedAt: time.Now(),
				Answers:   make([]models.UserAnswer, 0),
			}
			assessment, err = s.repo.CreateUserAssessment(newAssessment)
			if err != nil {
				log.Printf("为用户 '%s' 创建新评估失败: %v", userID, err)
				return nil, nil, errors.New("开始新评估失败，请稍后再试")
			}
			log.Printf("为用户 '%s' 创建新评估成功，ID: %d", userID, assessment.ID)
			// 新评估，CurrentQuestionID 为空，将从第一个问题开始
		} else { // 获取评估时发生其他错误
			log.Printf("获取用户 '%s' 的进行中评估失败: %v", userID, err)
			return nil, nil, errors.New("获取评估信息失败，请稍后再试")
		}
	} else {
		log.Printf("用户 '%s' 继续进行中的评估，ID: %d, 当前记录的问题ID: %s", userID, assessment.ID, assessment.CurrentQuestionID)
	}

	// 确定下一个问题
	var nextQuestionToShow *models.AssessmentQuestion

	if assessment.Status == models.AssessmentStatusCompleted || assessment.Status == models.AssessmentStatusCancelled {
		log.Printf("用户 '%s' 的评估 (ID: %d) 状态为 %s，不返回问题。", userID, assessment.ID, assessment.Status)
		return nil, assessment, nil // 评估已结束，不返回问题
	}

	if assessment.CurrentQuestionID == "" { // 全新评估或上次未记录当前问题
		if len(s.questions) > 0 {
			nextQuestionToShow = &s.questions[0]
			assessment.CurrentQuestionID = nextQuestionToShow.ID
		} else {
			log.Println("错误: 评估服务中的问题列表为空！")
			assessment.Status = models.AssessmentStatusCompleted // 异常情况
			_, _ = s.repo.UpdateUserAssessment(assessment)       // 尝试更新状态
			return nil, assessment, errors.New("系统错误：评估问卷未配置")
		}
	} else { // 有 CurrentQuestionID，判断是返回当前还是下一个
		currentQFromDef, exists := s.questionsByID[assessment.CurrentQuestionID]
		if !exists {
			log.Printf("错误: 评估 %d 的 CurrentQuestionID '%s' 在问卷定义中未找到！可能问卷已更新。将尝试从头开始。", assessment.ID, assessment.CurrentQuestionID)
			if len(s.questions) > 0 {
				nextQuestionToShow = &s.questions[0]
				assessment.CurrentQuestionID = nextQuestionToShow.ID
			} else { /* ... 同上，问卷为空的错误处理 ... */ 
				return nil, assessment, errors.New("系统错误：评估问卷未配置")
			}
		} else {
			// 检查当前 CurrentQuestionID 是否已回答
			answeredCurrent := false
			for _, ans := range assessment.Answers {
				if ans.QuestionID == assessment.CurrentQuestionID {
					answeredCurrent = true
					break
				}
			}

			if answeredCurrent { // 如果 CurrentQuestionID 已回答，则找它的下一个问题
				foundCurrentInList := false
				for i, qDef := range s.questions {
					if qDef.ID == assessment.CurrentQuestionID {
						foundCurrentInList = true
						if i+1 < len(s.questions) { // 如果有下一个问题
							nextQ := s.questions[i+1]
							nextQuestionToShow = &nextQ
							assessment.CurrentQuestionID = nextQ.ID
						} else { // 是最后一个问题且已回答，则评估完成
							assessment.Status = models.AssessmentStatusCompleted
							now := time.Now()
							assessment.CompletedAt = &now
							assessment.CurrentQuestionID = "" // 清空
						}
						break
					}
				}
				if !foundCurrentInList { // 理论上不会发生，因为 currentQFromDef 已经找到了
					log.Printf("严重错误: 在有序问题列表中未找到 CurrentQuestionID %s", assessment.CurrentQuestionID)
					return nil, assessment, errors.New("系统内部错误，请联系管理员")
				}
			} else { // 如果 CurrentQuestionID 未回答，则应该返回 CurrentQuestionID 本身
				nextQuestionToShow = currentQFromDef
				// assessment.CurrentQuestionID 不需要改变
			}
		}
	}

	// 更新评估状态（主要是 CurrentQuestionID 和可能的 Status, CompletedAt）
	updatedAssessment, err := s.repo.UpdateUserAssessment(assessment)
	if err != nil {
		log.Printf("为用户 '%s' 更新评估 %d 状态失败 (StartOrContinue): %v", userID, assessment.ID, err)
		// 返回原始 assessment 和下一个问题（如果已确定），让上层决定如何处理
		if nextQuestionToShow != nil {
			return nextQuestionToShow, assessment, errors.New("处理评估进度时遇到问题，请稍后再试")
		}
		return nil, assessment, errors.New("处理评估进度时遇到问题，请稍后再试")
	}

	if updatedAssessment.Status == models.AssessmentStatusCompleted || updatedAssessment.Status == models.AssessmentStatusCancelled {
		log.Printf("用户 '%s' 的评估 (ID: %d) 在 StartOrContinue 后状态为 %s。", userID, updatedAssessment.ID, updatedAssessment.Status)
		return nil, updatedAssessment, nil // 没有下一个问题了
	}
	
	if nextQuestionToShow == nil { // 如果逻辑走到这里 nextQuestionToShow 还是 nil，说明有问题
	    log.Printf("错误: StartOrContinueAssessment 逻辑未能确定下一个问题，评估ID: %d", updatedAssessment.ID)
	    return nil, updatedAssessment, errors.New("无法确定下一个评估问题，请重试或联系支持。")
	}

	log.Printf("为用户 '%s' (评估ID: %d) 返回下一个问题: ID=%s", userID, updatedAssessment.ID, nextQuestionToShow.ID)
	return nextQuestionToShow, updatedAssessment, nil
}


// SubmitAnswer (逻辑已优化)
func (s *assessmentService) SubmitAnswer(userID string, questionID string, answerValues []string) (*models.AssessmentQuestion, *models.UserAssessment, error) {
	assessment, err := s.repo.GetUserAssessmentByUserID(userID, models.AssessmentStatusInProgress)
	if err != nil || assessment == nil {
		log.Printf("提交答案失败：用户 '%s' 没有正在进行的评估，或获取失败: %v", userID, err)
		return nil, nil, errors.New("您当前没有正在进行的评估。如果想开始，请告诉我。")
	}

	if assessment.CurrentQuestionID == "" && questionID == "q_welcome" {
		// 特殊处理：如果 CurrentQuestionID 为空（可能是新评估刚开始），且提交的是对 q_welcome 的回答
		log.Printf("用户 '%s' 提交对欢迎问题 '%s' 的回答。", userID, questionID)
		assessment.CurrentQuestionID = questionID // 将当前问题设置为欢迎问题，以便后续逻辑正确处理
	} else if assessment.CurrentQuestionID != questionID {
		log.Printf("警告: 用户 '%s' 尝试回答问题 '%s'，但当前应答问题是 '%s'", userID, questionID, assessment.CurrentQuestionID)
		currentQ, exists := s.questionsByID[assessment.CurrentQuestionID]
		if !exists || currentQ == nil { // 添加nil检查
			log.Printf("错误：在SubmitAnswer中找不到当前应答问题定义 '%s'", assessment.CurrentQuestionID)
			return nil, assessment, fmt.Errorf("系统错误：找不到您当前应该回答的问题。请尝试说“继续评估”。")
		}
		return currentQ, assessment, fmt.Errorf("您似乎回答了错误的问题。当前应该回答的是：'%s'", currentQ.Text)
	}

	questionDef, exists := s.questionsByID[questionID]
	if !exists || questionDef == nil { // 添加nil检查
		log.Printf("错误: 提交答案失败，问题ID '%s' 未在问卷定义中找到。", questionID)
		return nil, assessment, errors.New("提交答案的问题无效，请重试")
	}

	if questionDef.IsRequired && (answerValues == nil || len(answerValues) == 0 || (len(answerValues) == 1 && strings.TrimSpace(answerValues[0]) == "")) {
		log.Printf("用户 '%s' 未回答必填问题 '%s'", userID, questionID)
		return questionDef, assessment, errors.New("这个问题是必答的哦，请提供您的回答。")
	}
	// ... (其他答案验证逻辑，如单选数量) ...

	// 特殊答案处理 (q_welcome, q_privacy_consent)
	userVisibleError := processSpecialAnswers(assessment, questionDef, answerValues)
	if userVisibleError != nil {
		_, _ = s.repo.UpdateUserAssessment(assessment) // 保存状态变更 (如 cancelled)
		return nil, assessment, userVisibleError
	}


	// 保存或更新答案
	answerFound := false
	for i, ans := range assessment.Answers {
		if ans.QuestionID == questionID {
			assessment.Answers[i].Answer = answerValues
			answerFound = true
			break
		}
	}
	if !answerFound {
		assessment.Answers = append(assessment.Answers, models.UserAnswer{
			QuestionID: questionID,
			Answer:     answerValues,
		})
	}

	// 确定下一个问题或完成
	var nextQuestionToShow *models.AssessmentQuestion
	currentIndexInList := -1
	for i, q := range s.questions { // s.questions 是有序的
		if q.ID == questionID {
			currentIndexInList = i
			break
		}
	}

	if currentIndexInList == -1 { /* ... 内部错误 ... */ 
	    return nil, assessment, errors.New("系统内部错误，无法定位问题顺序")
	}

	if currentIndexInList+1 < len(s.questions) {
		nextQ := s.questions[currentIndexInList+1]
		nextQuestionToShow = &nextQ
		assessment.CurrentQuestionID = nextQ.ID
		// Status 保持 InProgress
	} else { // 已是最后一个问题
		assessment.Status = models.AssessmentStatusCompleted
		now := time.Now()
		assessment.CompletedAt = &now
		assessment.CurrentQuestionID = "" // 清空
		log.Printf("用户 '%s' 评估 (ID: %d) 已完成所有问题。", userID, assessment.ID)
	}

	updatedAssessment, err := s.repo.UpdateUserAssessment(assessment)
	if err != nil {
		log.Printf("为用户 '%s' 更新评估 %d (提交答案 '%s' 后) 失败: %v", userID, assessment.ID, questionID, err)
		return questionDef, assessment, errors.New("保存您的回答时遇到问题，请稍后再试")
	}

	if updatedAssessment.Status == models.AssessmentStatusCompleted {
		// triggerPostAssessmentActions(updatedAssessment) // 触发评估后动作
		return nil, updatedAssessment, nil // 评估完成
	}

	log.Printf("为用户 '%s' (评估ID: %d) 在回答问题 '%s' 后，返回下一个问题: ID=%s", userID, updatedAssessment.ID, questionID, nextQuestionToShow.ID)
	return nextQuestionToShow, updatedAssessment, nil
}

// processSpecialAnswers 处理特殊答案并可能修改评估状态，返回用户可见的错误/消息
func processSpecialAnswers(assessment *models.UserAssessment, question *models.AssessmentQuestion, answerValues []string) error {
	if question.ID == "q_welcome" && contains(answerValues, "不了，下次吧") {
		assessment.Status = models.AssessmentStatusCancelled
		assessment.CurrentQuestionID = ""
		log.Printf("用户 '%s' 在欢迎问题选择“不了，下次吧”，评估ID %d 取消。", assessment.UserID, assessment.ID)
		return errors.New("好的，我们尊重您的选择。如果您之后改变主意，可以随时重新开始评估。")
	}
	if question.ID == "q_privacy_consent" && contains(answerValues, "我不同意") {
		assessment.Status = models.AssessmentStatusCancelled
		assessment.CurrentQuestionID = ""
		log.Printf("用户 '%s' 不同意隐私条款，评估ID %d 取消。", assessment.UserID, assessment.ID)
		return errors.New("我们非常重视您的隐私。如果您不同意处理您的信息，我们将无法进行后续的个性化服务。评估已中止。")
	}
	return nil
}


// GetAssessmentResult (保持不变)
func (s *assessmentService) GetAssessmentResult(userID string) (*models.UserAssessment, error) {
	// ... (实现)
	assessment, err := s.repo.GetUserAssessmentByUserID(userID, models.AssessmentStatusCompleted)
	if err != nil || assessment == nil {
		log.Printf("获取用户 '%s' 已完成的评估结果失败: %v", userID, err)
		return nil, errors.New("您尚未完成任何评估，或获取结果时发生错误。")
	}
	return assessment, nil
}

// contains (保持不变)
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}