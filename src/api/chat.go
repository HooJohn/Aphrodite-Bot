package api

import (
	"fmt"
	"log"
	"net/http"
	"project/config"
	"project/models"
	"project/repository"
	"project/services"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ClientChatRequest (保持不变)
type ClientChatRequest struct {
	UserID  string `json:"user_id" binding:"required"`
	Message string `json:"message" binding:"required"`
	GroupID string `json:"group_id,omitempty"`
}

// Constants for Agent IDs
const ProfileAssessmentAgentID = "hs_profile_assessment_agent"

// ChatHandler (已重构)
func ChatHandler(c *gin.Context) {
	var clientReq ClientChatRequest
	if err := c.ShouldBindJSON(&clientReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效: " + err.Error()})
		return
	}

	log.Printf("接收到来自用户 '%s' (群组 '%s') 的消息: '%s'", clientReq.UserID, clientReq.GroupID, clientReq.Message)

	// --- 1. 初始化服务和仓库 ---
	chatRepo := repository.NewChatRepository() // 直接使用 NewChatRepository
	assessmentRepo := repository.NewAssessmentRepository()
	assessmentService := services.NewAssessmentService(assessmentRepo)
	// SchedulerService 需要 AssessmentRepository 来检查评估状态
	schedulerService := services.NewSchedulerService(assessmentRepo)
	chatService := services.NewChatService()


	// --- 2. 保存用户消息 ---
	userChatMessage := models.ChatMessage{
		UserID:    clientReq.UserID,
		Role:      "user",
		Name:      clientReq.UserID, // 或用户昵称
		Content:   clientReq.Message,
		Timestamp: time.Now(),
	}
	if err := chatRepo.SaveMessage(userChatMessage); err != nil {
		log.Printf("严重错误: 保存用户消息失败 for user '%s': %v", clientReq.UserID, err)
		// 考虑是否需要向客户端返回错误，如果消息无法保存，后续流程可能无意义
		// 但为了保持流式，这里先记录错误，尝试继续，但要意识到历史可能不完整
		// 或者，如果这是一个关键步骤，则应该：
		// c.JSON(http.StatusInternalServerError, gin.H{"error": "系统内部错误，请稍后再试 (CM001)"})
		// return
	} else {
		log.Printf("用户 '%s' 的消息 (ID: %d) 已保存。", clientReq.UserID, userChatMessage.ID)
	}


	// --- 3. 获取聊天历史 (不包含当前刚发送的用户消息，供 Scheduler 使用) ---
	// SchedulerService.ScheduleAIResponses 的 history 参数应该是 *不* 包含当前用户消息的
	completeHistory, _ := chatRepo.GetMessagesByUserID(clientReq.UserID) // 忽略错误，空历史也可接受
	var historyForScheduler []models.ChatMessage
	if len(completeHistory) > 1 { // 如果历史中至少有两条（即当前用户消息和至少一条之前的）
		historyForScheduler = completeHistory[:len(completeHistory)-1]
	}


	// --- 4. 核心流程：判断是否处于评估阶段，并与 AssessmentService 交互 或 调用 Scheduler ---
	var assessmentQuestionForAgent *models.AssessmentQuestion
	var currentAssessmentStatus    models.UserAssessmentStatus
	var assessmentUserVisibleError error
	var nextAgentIDForChat         string
	var additionalContextHeader    string // 用于向LLM注入当前问题等上下文


	// 检查用户是否正在进行评估 (通过 SchedulerService 内部的检查)
	// 或者，ChatHandler 可以自己先查一下，再把状态告诉 SchedulerService
	// 我们让 SchedulerService 自己查（它已经注入了 assessmentRepo）
	
	availableAIs := getAvailableAIs(clientReq.GroupID, config.AppConfig)
	if len(availableAIs) == 0 {
		sendErrorEvent(c, "当前群组没有可用的AI成员。", "handler_setup")
		return
	}

	// 调用 SchedulerService (它现在会检查评估状态)
	// 注意：ScheduleAIResponses 的 userID 参数是新增的
	selectedAI_IDs, schedulerErr := schedulerService.ScheduleAIResponses(clientReq.UserID, clientReq.Message, historyForScheduler, availableAIs)
	if schedulerErr != nil {
		log.Printf("错误: AI调度失败 for user '%s': %v", clientReq.UserID, schedulerErr)
		sendErrorEvent(c, schedulerErr.Error(), "scheduler")
		return
	}
	if len(selectedAI_IDs) == 0 {
		log.Printf("信息: 没有AI被调度来回复用户 '%s'", clientReq.UserID)
		sendInfoEvent(c, "no_ai_scheduled", "抱歉，暂时没有AI可以响应该请求。")
		return
	}
	nextAgentIDForChat = selectedAI_IDs[0] // MVP: 取第一个
	log.Printf("用户 '%s' 的消息调度给AI: %s", clientReq.UserID, nextAgentIDForChat)


	// 如果选中的是评估专员，则需要与 AssessmentService 交互
	if nextAgentIDForChat == ProfileAssessmentAgentID {
		log.Printf("AI %s 被选中，处理评估逻辑 for user %s", ProfileAssessmentAgentID, clientReq.UserID)
		var tempAssessment *models.UserAssessment // 用于接收 AssessmentService 返回的评估对象

		// 判断是开始/继续评估，还是提交答案
		// 这可以通过检查用户有无“进行中”的评估，以及该评估的 CurrentQuestionID
		activeAssessment, _ := assessmentRepo.GetUserAssessmentByUserID(clientReq.UserID, models.AssessmentStatusInProgress)

		if activeAssessment != nil && activeAssessment.CurrentQuestionID != "" {
			// 用户有进行中的评估，且有当前待回答问题，说明用户的 clientReq.Message 是对这个问题的回答
			log.Printf("用户 %s 正在回答评估问题 ID: %s", clientReq.UserID, activeAssessment.CurrentQuestionID)
			assessmentQuestionForAgent, tempAssessment, assessmentUserVisibleError = assessmentService.SubmitAnswer(
				clientReq.UserID,
				activeAssessment.CurrentQuestionID,
				[]string{clientReq.Message},
			)
		} else {
			// 用户没有进行中的评估，或者评估的 CurrentQuestionID 为空，表示开始新评估或从欢迎语继续
			log.Printf("用户 %s 开始或继续评估流程", clientReq.UserID)
			assessmentQuestionForAgent, tempAssessment, assessmentUserVisibleError = assessmentService.StartOrContinueAssessment(clientReq.UserID)
		}
		
		if tempAssessment != nil { // 更新当前评估状态
		    currentAssessmentStatus = tempAssessment.Status
		}


		if assessmentUserVisibleError != nil {
			log.Printf("AssessmentService 为用户 '%s' 返回特定信息/错误: %v", clientReq.UserID, assessmentUserVisibleError)
			sendErrorEvent(c, assessmentUserVisibleError.Error(), ProfileAssessmentAgentID) // 由评估专员名义发出

			// 保存这个“错误”或“状态”消息作为AI的回复
			if tempAssessment != nil { // 确保有关联的评估对象
				saveAIMessage(chatRepo, clientReq.UserID, ProfileAssessmentAgentID, assessmentUserVisibleError.Error())
			}
			return // 评估流程因错误或用户选择而中止
		}

		// 准备 additionalContextHeader
		if assessmentQuestionForAgent != nil {
			additionalContextHeader = fmt.Sprintf("\n\n[系统指令：当前评估问题]\n问题文本：%s", assessmentQuestionForAgent.Text)
			if len(assessmentQuestionForAgent.Options) > 0 {
				additionalContextHeader += "\n选项（请选择一项或多项）：\n"
				for i, opt := range assessmentQuestionForAgent.Options {
					additionalContextHeader += fmt.Sprintf("%c. %s\n", 'A'+i, opt)
				}
			}
			log.Printf("为评估专员准备的问题上下文: %.100s...", additionalContextHeader)
		} else if currentAssessmentStatus == models.AssessmentStatusCompleted {
			additionalContextHeader = "\n\n[系统指令：评估已完成]\n请按照你的角色设定，告知用户评估已完成，并提示后续流程。"
			log.Printf("为评估专员准备的评估完成上下文。")
		} else if currentAssessmentStatus == models.AssessmentStatusCancelled {
             // 这个情况通常 assessmentUserVisibleError 已经处理了，但作为双重保险
            additionalContextHeader = "\n\n[系统指令：评估已中止]\n请按照你的角色设定，礼貌告知用户评估已中止。"
            log.Printf("为评估专员准备的评估中止上下文。")
        }

	}
	// else { 调度器选择了其他AI，additionalContextHeader 为空 }


	// --- 5. 准备并执行AI回复 ---
	var selectedAIConfig *config.LLMCharacter
	for _, char := range config.AppConfig.LLMCharacters {
		if char.ID == nextAgentIDForChat {
			selectedAIConfig = char
			break
		}
	}
	if selectedAIConfig == nil {
		log.Printf("严重错误: 无法找到选中AI '%s' 的配置信息", nextAgentIDForChat)
		sendErrorEvent(c, "系统内部错误，无法处理AI响应配置。", "handler_config")
		return
	}

	// 构建 ChatService 的 ChatRequest
	// 传给 ChatService.ProcessMessageStream 的 history 应该是包含当前用户消息的完整历史
	// 因为 ProcessMessageStream 内部会处理历史截取和与 currentUserMessage 的组合
	serviceReq := services.ChatRequest{
		Message:      clientReq.Message, // 用户原始消息文本，ChatService可能用到
		UserID:       clientReq.UserID,
		Model:        selectedAIConfig.Model,
		CustomPrompt: selectedAIConfig.CustomPrompt,
		AIName:       selectedAIConfig.Name,
		History:      completeHistory, // 传入包含当前用户消息的完整历史
		Index:        0, // 让 ChatService 知道 currentUserMessage 是最新的
	}

	// 调用服务层处理流式业务逻辑
	fullAIReply, err := chatService.ProcessMessageStream(userChatMessage, serviceReq, c.Writer, additionalContextHeader)

	if err != nil {
		log.Printf("错误: 处理AI '%s' 的流式消息失败: %v", selectedAIConfig.Name, err)
		// 错误可能已在 ProcessMessageStream 内部通过 SSE 发送，或在此处发送
		// 这里不再重复发送SSE错误，因为ProcessMessageStream应该已经处理或返回错误让这里处理
		// 但如果 ProcessMessageStream 只是返回error，没有发SSE，则这里需要发
		// 我们假设 ProcessMessageStream 内部不发送错误SSE，仅返回错误
		if !isStreamClosedError(err) { // 避免在连接已关闭时尝试写入
			sendErrorEvent(c, fmt.Sprintf("AI %s 响应时遇到问题，请稍后再试。", selectedAIConfig.Name), selectedAIConfig.Name)
		}
		return
	}

	// --- 6. 保存 AI 回复 ---
	if strings.TrimSpace(fullAIReply) != "" { // 只有非空回复才保存
		saveAIMessage(chatRepo, clientReq.UserID, selectedAIConfig.Name, fullAIReply)
	} else {
		log.Printf("信息: AI '%s' 为用户 '%s' 生成了空回复，未保存。", selectedAIConfig.Name, clientReq.UserID)
	}

	log.Printf("ChatHandler for user '%s' with AI '%s' completed successfully.", clientReq.UserID, selectedAIConfig.Name)
}


// getAvailableAIs 辅助函数
func getAvailableAIs(groupID string, appCfg config.Config) []*config.LLMCharacter {
	var availableAIs []*config.LLMCharacter
	targetGroupID := groupID
	if targetGroupID == "" && len(appCfg.LLMGroups) > 0 {
		targetGroupID = appCfg.LLMGroups[0].ID
	}

	for _, group := range appCfg.LLMGroups {
		if group.ID == targetGroupID {
			for _, memberID := range group.Members {
				foundChar := false
				for _, char := range appCfg.LLMCharacters {
					if char.ID == memberID {
						availableAIs = append(availableAIs, char)
						foundChar = true
						break
					}
				}
				if !foundChar {
				    log.Printf("警告: 群组成员 AI ID '%s' 在 LLMCharacters 中未找到定义。", memberID)
				}
			}
			break
		}
	}
	return availableAIs
}

// saveAIMessage 辅助函数，用于保存AI消息
func saveAIMessage(repo repository.ChatRepository, userID, aiName, content string) {
	aiChatMessage := models.ChatMessage{
		UserID:    userID,
		Role:      "assistant",
		Name:      aiName,
		Content:   content,
		Timestamp: time.Now(),
	}
	if err := repo.SaveMessage(aiChatMessage); err != nil {
		log.Printf("严重错误: 保存AI '%s' 的回复失败 for user '%s': %v", aiName, userID, err)
	} else {
		log.Printf("AI '%s' 的回复 (ID: %d) 已为用户 '%s' 保存.", aiName, aiChatMessage.ID, userID)
	}
}

// sendErrorEvent 辅助函数，用于向客户端发送错误SSE事件
func sendErrorEvent(c *gin.Context, errorMsg string, sourceAgentName string) {
	// 确保 SSE header 已设置，如果之前没有数据发送过
	if c.Writer.Header().Get("Content-Type") == "" {
		c.Writer.Header().Set("Content-Type", "text/event-stream")
		c.Writer.Header().Set("Cache-Control", "no-cache")
		c.Writer.Header().Set("Connection", "keep-alive")
	}
	data := fmt.Sprintf("data: {\"error\": \"%s\", \"ai_name\": \"%s\"}\n\n", escapeJSONString(errorMsg), sourceAgentName)
	_, err := c.Writer.Write([]byte(data))
	if err == nil {
		c.Writer.Flush()
	} else {
		log.Printf("写入错误SSE事件到客户端失败: %v", err)
	}
}

// sendInfoEvent 辅助函数，用于向客户端发送通用信息SSE事件
func sendInfoEvent(c *gin.Context, eventType string, message string) {
	if c.Writer.Header().Get("Content-Type") == "" {
		c.Writer.Header().Set("Content-Type", "text/event-stream")
		c.Writer.Header().Set("Cache-Control", "no-cache")
		c.Writer.Header().Set("Connection", "keep-alive")
	}
	data := fmt.Sprintf("data: {\"event\": \"%s\", \"message\": \"%s\"}\n\n", eventType, escapeJSONString(message))
	_, err := c.Writer.Write([]byte(data))
	if err == nil {
		c.Writer.Flush()
	} else {
		log.Printf("写入信息SSE事件到客户端失败: %v", err)
	}
}


// isStreamClosedError 检查错误是否表示流已关闭或网络问题
func isStreamClosedError(err error) bool {
    if err == nil {
        return false
    }
    // 常见的网络错误或流关闭错误，根据实际情况补充
    errStr := strings.ToLower(err.Error())
    return strings.Contains(errStr, "broken pipe") ||
           strings.Contains(errStr, "connection reset by peer") ||
           strings.Contains(errStr, "stream closed") ||
           strings.Contains(errStr, "client disconnected") // 根据实际错误添加
}

// escapeJSONString (保持不变)
func escapeJSONString(s string) string {
    s = strings.ReplaceAll(s, "\\", "\\\\")
    s = strings.ReplaceAll(s, "\"", "\\\"")
    s = strings.ReplaceAll(s, "\n", "\\n")
    return s
}