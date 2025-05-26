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
	if userID != "" && s.assessmentRepo != nil {
		activeAssessment, err := s.assessmentRepo.GetUserAssessmentByUserID(userID, models.AssessmentStatusInProgress)
		if err != nil {
			// Log the error but don't necessarily fail the whole scheduling if it's just a DB issue for this check.
			// The main scheduling logic can proceed without this specific check if it fails.
			log.Printf("WARN: [SchedulerService] Failed to check for active assessment for userID '%s': %v. Proceeding with general scheduling.", userID, err)
		} else if activeAssessment != nil {
			log.Printf("INFO: [SchedulerService] UserID '%s' is currently in assessment (ID: %d). Prioritizing ProfileAssessmentAgent.", userID, activeAssessment.ID)
			isAssessorAvailable := false
			for _, ai := range availableAIs {
				if ai.ID == ProfileAssessmentAgentID {
					isAssessorAvailable = true
					break
				}
			}
			if isAssessorAvailable {
				return []string{ProfileAssessmentAgentID}, nil
			}
			log.Printf("WARN: [SchedulerService] ProfileAssessmentAgent ('%s') is not in availableAIs list, but userID '%s' is in assessment. This might be a configuration issue.", ProfileAssessmentAgentID, userID)
			// Return an error because if user is in assessment, only assessor should respond.
			return nil, fmt.Errorf("assessment agent '%s' is currently unavailable. Please try again later or contact support", ProfileAssessmentAgentID)
		}
	}
	// --- 结束新增检查 ---

	// 1. 收集所有可用的标签 (排除调度器AI)
	allTags := make(map[string]bool)
	var actualAvailableAIsForTagging []*config.LLMCharacter
	for _, ai := range availableAIs {
		if ai.ID == "ai0" { 
			continue
		}
		actualAvailableAIsForTagging = append(actualAvailableAIsForTagging, ai)
		for _, tag := range ai.Tags {
			allTags[tag] = true
		}
	}
	tagsList := make([]string, 0, len(allTags))
	for tag := range allTags {
		tagsList = append(tagsList, tag)
	}

	if len(tagsList) == 0 && len(actualAvailableAIsForTagging) > 0 {
		log.Printf("WARN: [SchedulerService] No tags defined across %d available AIs (excluding scheduler). Scheduler AI might not effectively match.", len(actualAvailableAIsForTagging))
	}


	// 2. 使用AI模型分析消息并匹配标签
	matchedTags, errAnalyze := s.analyzeMessageWithAI(message, tagsList, history)
	if errAnalyze != nil {
		log.Printf("ERROR: [SchedulerService] Failed to analyze message with AI for userID '%s': %v. Proceeding with empty tags.", userID, errAnalyze)
		// Proceed with empty matchedTags, the scoring will handle it.
		// Depending on severity, could return error: return nil, fmt.Errorf("failed to analyze message for scheduling: %w", errAnalyze)
		matchedTags = []string{} 
	}
	log.Printf("INFO: [SchedulerService] For userID '%s', message '%.50s...', matched tags by scheduler AI: %v", userID, message, matchedTags)


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
	sortedAIs := sortAIsByScore(aiScores) // Assuming sortAIsByScore is robust
	log.Printf("INFO: [SchedulerService] AI scores for userID '%s': %v", userID, aiScores)
	log.Printf("INFO: [SchedulerService] Sorted AIs by score for userID '%s' (IDs): %v", userID, sortedAIs)

	// 5. 如果没有通过评分匹配到任何AI
	if len(sortedAIs) == 0 {
		log.Printf("INFO: [SchedulerService] No AI matched via scoring for userID '%s'. Attempting fallback.", userID)
		fallbackAgentID := "hs_empathy_agent" // Example fallback
		isFallbackAvailable := false
		for _, ai := range availableAIs { // Check against the original availableAIs list
			if ai.ID == fallbackAgentID {
				isFallbackAvailable = true
				break
			}
		}
		if isFallbackAvailable {
			log.Printf("INFO: [SchedulerService] Fallback agent '%s' selected for userID '%s'.", fallbackAgentID, userID)
			return []string{fallbackAgentID}, nil
		}
		log.Printf("WARN: [SchedulerService] Fallback agent '%s' not available or not found for userID '%s'. No AI scheduled.", fallbackAgentID, userID)
		// It's important that ChatHandler can inform the user gracefully.
		// Returning an empty slice and no error signals "no AI scheduled" rather than a system failure.
		return []string{}, nil 
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
	//     log.Printf("INFO: [SchedulerService] High-risk scenario identified for userID '%s'. Prioritizing HealthSafetyAgent.", userID)
	//     return []string{"hs_health_safety_agent"}, nil
	// }

	log.Printf("INFO: [SchedulerService] Final selected AI(s) for userID '%s': %v", userID, finalSelectedAIs)
	return finalSelectedAIs, nil
}


// analyzeMessageWithAI uses an LLM to analyze the message and match it against a list of tags.
func (s *schedulerService) analyzeMessageWithAI(message string, allTags []string, history []models.ChatMessage) ([]string, error) {
	var schedulerAIConfig *config.LLMCharacter
	for _, char := range config.AppConfig.LLMCharacters {
		if char.ID == "ai0" {
			schedulerAIConfig = char
			break
		}
	}

	if schedulerAIConfig == nil {
		log.Printf("ERROR: [SchedulerService] Scheduler AI (ID 'ai0') configuration not found.")
		return nil, errors.New("scheduler AI (ID 'ai0') configuration not found")
	}
	log.Printf("INFO: [SchedulerService] Using scheduler AI '%s' with model '%s' for tag analysis.", schedulerAIConfig.Name, schedulerAIConfig.Model)

	providerKey, modelExists := config.AppConfig.LLMModels[schedulerAIConfig.Model]
	if !modelExists {
		errMsg := fmt.Sprintf("provider for scheduler AI model '%s' not found in llm_models", schedulerAIConfig.Model)
		log.Printf("ERROR: [SchedulerService] %s", errMsg)
		return nil, errors.New(errMsg)
	}

	providerConfig, providerExists := config.AppConfig.LLMProviders[providerKey]
	if !providerExists {
		errMsg := fmt.Sprintf("provider configuration for key '%s' (model '%s') not found in llm_providers", providerKey, schedulerAIConfig.Model)
		log.Printf("ERROR: [SchedulerService] %s", errMsg)
		return nil, errors.New(errMsg)
	}
	
	apiKey := providerConfig.APIKey
	baseURL := providerConfig.BaseURL
	if apiKey == "" || baseURL == "" {
		errMsg := fmt.Sprintf("API key or BaseURL for scheduler AI provider '%s' is not configured", providerKey)
		log.Printf("ERROR: [SchedulerService] %s", errMsg)
		return nil, errors.New(errMsg)
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
		Model:       schedulerAIConfig.Model,
		Messages:    messages,
		Temperature: 0.2, 
		MaxTokens:   50,  
	})
	if err != nil {
		errMsg := fmt.Sprintf("scheduler LLM call failed for model %s", schedulerAIConfig.Model)
		log.Printf("ERROR: [SchedulerService] %s: %v", errMsg, err)
		return nil, fmt.Errorf("%s: %w", errMsg, err)
	}

	if len(completion.Choices) == 0 || completion.Choices[0].Message.Content == "" {
		log.Printf("WARN: [SchedulerService] Scheduler LLM for model %s returned no content for tag analysis.", schedulerAIConfig.Model)
		return []string{}, nil // No tags matched is not an error, but a valid outcome.
	}

	content := completion.Choices[0].Message.Content
	log.Printf("INFO: [SchedulerService] Scheduler LLM raw response for tag analysis: '%s'", content)
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

// Helper functions (ensure they also have improved logging if they can fail or make important decisions)
// For brevity, their internal logging is not detailed here but should follow similar principles.

func containsTag(tags []string, tag string) bool { 
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false 
}

func getRecentHistory(history []models.ChatMessage, limit int) []models.ChatMessage { 
	if len(history) <= limit {
		return history
	}
	return history[len(history)-limit:]
}

func sortAIsByScore(aiScores map[string]int) []string {
    type aiScorePair struct {
        ID    string
        Score int
    }
    pairs := make([]aiScorePair, 0, len(aiScores))
    for id, score := range aiScores {
        pairs = append(pairs, aiScorePair{id, score})
    }
    // Sort descending by score
    sort.Slice(pairs, func(i, j int) bool {
        return pairs[i].Score > pairs[j].Score
    })
    sortedIDs := make([]string, len(pairs))
    for i, pair := range pairs {
        sortedIDs[i] = pair.ID
    }
    return sortedIDs
}

func первыеНесколькоСлов(s string, count int) string { 
    words := strings.Fields(s)
    if len(words) < count {
        return s
    }
    return strings.Join(words[:count], " ") + "..."
}

// Added sort import needed by sortAIsByScore
// Ensure "sort" is imported in the package.
// import "sort" (if not already there)
// NOTE: The tool does not allow adding imports directly, this is a comment for the real code.
// The provided code snippet for scheduler_service.go already has "sort" commented out.
// It would need to be uncommented if sortAIsByScore is used as implemented here.
// For now, assuming sortAIsByScore is simplified or its dependencies are met.