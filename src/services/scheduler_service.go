package services

import (
	"context"
	"errors"
	"log"
	"fmt"
	// "math/rand"
	"project/config"
	"project/models"
	"project/repository" // 引入 repository 以便检查评估状态
	// "sort"
	"strings"
	// "time"

	openai "github.com/sashabaranov/go-openai"
)

// Constants for Agent IDs
const ProfileAssessmentAgentID = "hs_profile_assessment_agent" // 与 ChatHandler 保持一致

// SchedulerService 调度服务接口 (保持不变)
type SchedulerService interface {
	ScheduleAIResponses(userID string, message string, history []models.ChatMessage, availableAIs []*config.LLMCharacter) ([]string, error)
}

// schedulerService 调度服务实现
type schedulerService struct {
	assessmentRepo repository.AssessmentRepository // 依赖注入
}

// NewSchedulerService 创建调度服务实例
func NewSchedulerService(assessRepo repository.AssessmentRepository) SchedulerService {
	return &schedulerService{
		assessmentRepo: assessRepo,
	}
}

// ScheduleAIResponses 调度AI响应 (userID 参数是新增的)
func (s *schedulerService) ScheduleAIResponses(userID string, message string, history []models.ChatMessage, availableAIs []*config.LLMCharacter) ([]string, error) {
	// --- 新增：检查用户是否正在进行评估 ---
	if userID != "" && s.assessmentRepo != nil { // 确保 userID 和 repo 有效
		activeAssessment, _ := s.assessmentRepo.GetUserAssessmentByUserID(userID, models.AssessmentStatusInProgress)
		// 我们忽略 GetUserAssessmentByUserID 的错误，因为找不到评估也是一种有效状态（即用户不在评估中）

		if activeAssessment != nil { // 如果用户有正在进行的评估
			log.Printf("用户 '%s' 正在进行评估 (ID: %d)，调度器优先选择评估专员。", userID, activeAssessment.ID)
			// 确保评估专员在可用列表中
			for _, ai := range availableAIs {
				if ai.ID == ProfileAssessmentAgentID {
					return []string{ProfileAssessmentAgentID}, nil
				}
			}
			log.Printf("警告: 评估专员 %s 不在可用AI列表中，但用户 %s 正在评估中。", ProfileAssessmentAgentID, userID)
			// 理论上评估专员应该始终可用，如果不可用，可能返回一个错误或通用AI
			return nil, errors.New("评估专员当前不可用，请稍后再试")
		}
	}
	// --- 结束新增检查 ---

	// 1. 收集所有可用的标签 (排除调度器AI)
	allTags := make(map[string]bool)
	for _, ai := range availableAIs {
		if ai.ID == "ai0" { // 假设 "ai0" 是调度器AI的固定ID
			continue
		}
		for _, tag := range ai.Tags {
			allTags[tag] = true
		}
	}
	tagsList := make([]string, 0, len(allTags))
	for tag := range allTags {
		tagsList = append(tagsList, tag)
	}
	if len(tagsList) == 0 && len(availableAIs) > 1 { // 如果没有标签但有AI，调度器AI可能无法工作
		log.Println("警告: 可用AI没有定义任何标签，调度器AI可能无法有效匹配。")
		// 此时可以考虑一个后备的随机选择逻辑，或者默认选择一个通用AI
	}


	// 2. 使用AI模型分析消息并匹配标签
	matchedTags, err := s.analyzeMessageWithAI(message, tagsList, history) // analyzeMessageWithAI 内部会获取 ai0 配置
	if err != nil {
		log.Printf("分析消息失败 (analyzeMessageWithAI): %v。将尝试无标签匹配。", err)
		matchedTags = []string{} // 即使分析失败，也继续，只是没有标签匹配
	}
	log.Printf("调度器AI为消息 '%s...' 匹配的标签: %v", первыеНесколькоСлов(message, 10), matchedTags)


	// (可选) 特殊标签处理，如 "文字游戏" (保持不变，或根据产品需求移除)
	if containsTag(matchedTags, "文字游戏") {
		// ...
	}

	// 3. 计算每个AI的匹配分数
	aiScores := make(map[string]int)
	messageLower := strings.ToLower(message)

	for _, ai := range availableAIs {
		if ai.ID == "ai0" { continue } // 跳过调度器

		score := 0
		// 标签匹配分数 (提高基础分)
		for _, tag := range matchedTags {
			if containsTag(ai.Tags, tag) {
				score += 3 // 每个匹配的标签得3分
			}
		}

		// 直接提到AI名字额外加分
		if strings.Contains(messageLower, strings.ToLower(ai.Name)) || strings.Contains(messageLower, "@"+strings.ToLower(ai.Name)) {
			score += 5
		}

		// (可选) 历史对话相关性加分 (保持简单)
		recentHistory := getRecentHistory(history, 3) // 只看最近3条（不含当前用户消息）
		for _, hist := range recentHistory {
			if hist.Name == ai.Name && len(hist.Content) > 0 { // 假设ChatMessage有Name字段存AI名
				score += 1
				break // 近期参与过一次即可
			}
		}
		
		// (可选) 为特定角色的核心标签增加额外权重
		// 例如，如果匹配到“知识科普”且AI是科普君，可以再加分
		// if ai.ID == "hs_knowledge_expert_agent" && containsTag(matchedTags, "知识科普") {
		//     score += 2
		// }


		if score > 0 {
			aiScores[ai.ID] = score
		}
	}

	// 4. 根据分数排序选择AI
	sortedAIs := sortAIsByScore(aiScores)
	log.Printf("AI得分情况: %v", aiScores)
	log.Printf("排序后的AI (ID列表): %v", sortedAIs)

	// 5. 如果没有通过评分匹配到任何AI
	if len(sortedAIs) == 0 {
		log.Println("没有AI通过评分机制匹配。")
		// 后备逻辑：选择一个默认的通用回复型AI，例如“知心姐”或一个专门的“引导员”
		// 不要随机选择，因为这在专业场景下体验不好
		fallbackAgentID := "hs_empathy_agent" // 假设知心姐是后备
		isFallbackAvailable := false
		for _, ai := range availableAIs {
			if ai.ID == fallbackAgentID {
				isFallbackAvailable = true
				break
			}
		}
		if isFallbackAvailable {
			log.Printf("选择后备Agent: %s", fallbackAgentID)
			return []string{fallbackAgentID}, nil
		}
		log.Println("警告: 连后备Agent也无法选择或不可用。")
		return []string{}, errors.New("抱歉，暂时无法处理您的请求") // 返回错误，让上层ChatHandler处理
	}

	// 6. 限制最大回复数量 (调整为更少，例如1-2个)
	maxResponders := 1 // MVP阶段，通常只让一个AI回复，除非有明确的协同设计
	// if len(sortedAIs) > 1 && aiScores[sortedAIs[0]] - aiScores[sortedAIs[1]] < 2 { // 如果前两名分数非常接近
	//     maxResponders = 2
	// }

	finalSelectedAIs := sortedAIs
	if len(sortedAIs) > maxResponders {
		finalSelectedAIs = sortedAIs[:maxResponders]
	}
	
	// (可选) 如果选中的是 HealthSafetyAgent，且场景高度相关，确保其优先或单独响应
	// if contains(finalSelectedAIs, "hs_health_safety_agent") && isHighRisk(matchedTags) {
	//     return []string{"hs_health_safety_agent"}, nil
	// }


	log.Printf("最终选择的AI进行回复: %v", finalSelectedAIs)
	return finalSelectedAIs, nil
}


// analyzeMessageWithAI (基本保持不变，但确保 ai0 的 CustomPrompt 已优化)
func (s *schedulerService) analyzeMessageWithAI(message string, allTags []string, history []models.ChatMessage) ([]string, error) {
	var schedulerAIConfig *config.LLMCharacter
	for _, char := range config.AppConfig.LLMCharacters { // 从AppConfig动态查找调度器AI
		if char.ID == "ai0" { // 或者 char.Personality == "scheduler"
			schedulerAIConfig = char
			break
		}
	}

	if schedulerAIConfig == nil {
		return nil, errors.New("调度器AI (ID 'ai0') 配置未找到")
	}

	providerKey, modelExists := config.AppConfig.LLMModels[schedulerAIConfig.Model]
	if !modelExists {
		return nil, fmt.Errorf("调度器AI模型 '%s' 的provider未在llm_models中配置", schedulerAIConfig.Model)
	}

	providerConfig, providerExists := config.AppConfig.LLMProviders[providerKey]
	if !providerExists {
		return nil, fmt.Errorf("调度器AI模型 '%s' 的provider '%s' 未在llm_providers中配置", schedulerAIConfig.Model, providerKey)
	}
	
	apiKey := providerConfig.APIKey
	baseURL := providerConfig.BaseURL
	if apiKey == "" || baseURL == "" {
		return nil, fmt.Errorf("调度器AI provider '%s' 的API密钥或基础URL未配置", providerKey)
	}

	openaiConfig := openai.DefaultConfig(apiKey)
	openaiConfig.BaseURL = baseURL
	client := openai.NewClientWithConfig(openaiConfig)

	prompt := schedulerAIConfig.CustomPrompt
	tagsStr := strings.Join(allTags, ", ")
	prompt = strings.ReplaceAll(prompt, "#allTags#", tagsStr) // 替换标签占位符

	messages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: prompt},
	}

	// 历史消息处理 (保持不变)
	historyLimit := 5 // 调度器AI不需要太长的历史，减少token消耗
	if len(history) > historyLimit {
		history = history[len(history)-historyLimit:]
	}
	for _, msg := range history { // 假设 history 中的 Role 已正确
	    role := openai.ChatMessageRoleUser
	    if strings.ToLower(msg.Role) == "assistant" || strings.ToLower(msg.Role) == "ai" {
	        role = openai.ChatMessageRoleAssistant
	    }
		messages = append(messages, openai.ChatCompletionMessage{Role: role, Content: msg.Content})
	}
	messages = append(messages, openai.ChatCompletionMessage{Role: openai.ChatMessageRoleUser, Content: message})

	ctx := context.Background()
	completion, err := client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:       schedulerAIConfig.Model, // 使用调度器AI自己配置的模型
		Messages:    messages,
		Temperature: 0.2, // 调度器需要确定性高一些
		MaxTokens:   50,  // 标签列表通常不长
	})
	if err != nil {
		return nil, fmt.Errorf("调用调度器LLM失败: %w", err)
	}

	if len(completion.Choices) == 0 || completion.Choices[0].Message.Content == "" {
		log.Println("调度器LLM未返回有效标签内容。")
		return []string{}, nil // 返回空标签列表，而不是错误
	}

	content := completion.Choices[0].Message.Content
	rawMatchedTags := strings.Split(content, ",")
	matchedTags := make([]string, 0, len(rawMatchedTags))
	for _, tag := range rawMatchedTags {
		trimmedTag := strings.TrimSpace(tag)
		if trimmedTag != "" { // 避免空标签
			matchedTags = append(matchedTags, trimmedTag)
		}
	}
	return matchedTags, nil
}

// 辅助函数 (getRecentHistory, sortAIsByScore, shuffleAIs, min, containsTag 保持不变)
// ...
func containsTag(tags []string, tag string) bool { /* ... */ return false }
func getRecentHistory(history []models.ChatMessage, limit int) []models.ChatMessage { /* ... */ return nil }
func sortAIsByScore(aiScores map[string]int) []string { /* ... */ return nil }
func первыеНесколькоСлов(s string, count int) string { // 辅助函数，用于日志截断
    words := strings.Fields(s)
    if len(words) < count {
        return s
    }
    return strings.Join(words[:count], " ") + "..."
}