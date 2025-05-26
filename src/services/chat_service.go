package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"project/config"
	"project/models"
	"strings"
	// "time"

	openai "github.com/sashabaranov/go-openai"
)

// ChatService 聊天服务接口
type ChatService interface {
	ProcessMessageStream(
		currentUserMessage models.ChatMessage,
		req ChatRequest,
		writer http.ResponseWriter,
		additionalContextHeader string, // 新增参数
	) (string, error) // 返回完整AI回复
	GetChatHistory(userID string) ([]models.ChatMessage, error) // 保持接口，但实现可能调整
}

// ChatRequest (保持不变)
type ChatRequest struct {
	Message      string `json:"message"` // 用户原始消息字符串
	UserID       string `json:"user_id"`
	Model        string `json:"model"`
	CustomPrompt string `json:"custom_prompt"`
	AIName       string `json:"aiName"`
	History      []models.ChatMessage `json:"history"` // 应该是纯粹的过去历史记录
	Index        int    `json:"index"`   // 通常为0，表示当前消息是最新
	// ResponseCallback func(string) // 当前未使用
}

type chatService struct{}

func NewChatService() ChatService {
	return &chatService{}
}

func (s *chatService) ProcessMessageStream(
	currentUserMessage models.ChatMessage, // 当前用户的消息对象
	req ChatRequest,
	writer http.ResponseWriter,
	additionalContextHeader string, // 接收此参数
) (string, error) {

	providerKey, modelExists := config.AppConfig.LLMModels[req.Model]
	if !modelExists { /* ... 错误处理 ... */ return "", errors.New("model not found") }
	providerConfig, providerExists := config.AppConfig.LLMProviders[providerKey]
	if !providerExists { /* ... 错误处理 ... */ return "", errors.New("provider not found") }
	apiKey := providerConfig.APIKey
	baseURL := providerConfig.BaseURL
	if apiKey == "" || baseURL == "" { /* ... 错误处理 ... */ return "", errors.New("api key or baseurl empty") }

	oclient := openai.DefaultConfig(apiKey)
	oclient.BaseURL = baseURL
	client := openai.NewClientWithConfig(oclient)

	systemPrompt := ""
	if req.CustomPrompt != "" {
		systemPrompt = req.CustomPrompt
		systemPrompt = strings.ReplaceAll(systemPrompt, "#name#", req.AIName)
		var currentGroupName string
		if len(config.AppConfig.LLMGroups) > 0 { currentGroupName = config.AppConfig.LLMGroups[0].Name } else { currentGroupName = "我们的群聊"}
		systemPrompt = strings.ReplaceAll(systemPrompt, "#groupName#", currentGroupName)
	} else if config.AppConfig.LLMSystemPrompt != "" {
		systemPrompt = strings.Replace(config.AppConfig.LLMSystemPrompt, "#name#", req.AIName, -1)
		// ... (同样替换 #groupName#)
	}

	var llmMessages []openai.ChatCompletionMessage
	if systemPrompt != "" {
		log.Printf("为AI '%s' 构建的System Prompt: %.100s...", req.AIName, systemPrompt)
		llmMessages = append(llmMessages, openai.ChatCompletionMessage{
			Role: openai.ChatMessageRoleSystem, Content: systemPrompt,
		})
	}

	if additionalContextHeader != "" {
		log.Printf("为AI '%s' 注入额外上下文头部: %.100s...", req.AIName, additionalContextHeader)
		llmMessages = append(llmMessages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleAssistant, // 作为AI接收到的任务指令
			Content: additionalContextHeader,
		})
	}

	// 添加历史消息 (req.History 是纯历史)
	historyLimit := 10 // 可配置
	actualHistory := req.History
	if len(actualHistory) > historyLimit {
		actualHistory = actualHistory[len(actualHistory)-historyLimit:]
	}
	for _, msg := range actualHistory {
		var openaiRole string
		switch strings.ToLower(msg.Role) {
		case "user": openaiRole = openai.ChatMessageRoleUser
		case "assistant", "ai": openaiRole = openai.ChatMessageRoleAssistant
		default: openaiRole = openai.ChatMessageRoleUser
		}
		if openaiRole == openai.ChatMessageRoleAssistant && msg.Name != "" {
			llmMessages = append(llmMessages, openai.ChatCompletionMessage{Role: openaiRole, Name: msg.Name, Content: msg.Content})
		} else {
			llmMessages = append(llmMessages, openai.ChatCompletionMessage{Role: openaiRole, Content: msg.Content})
		}
	}

	// 添加当前用户消息 (来自 currentUserMessage 对象)
	llmMessages = append(llmMessages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser, // currentUserMessage.Role 应该是 "user"
		Content: currentUserMessage.Content,
	})
	
	// 打印最终发送给LLM的messages (部分内容，用于调试)
	// for idx, m := range llmMessages {
	//    log.Printf("LLM Message [%d] Role: %s, Name: %s, Content: %.50s...", idx, m.Role, m.Name, m.Content)
	// }


	ctx := context.Background()
	chatAPIReq := openai.ChatCompletionRequest{
		Model:    req.Model,
		Messages: llmMessages,
		Stream:   true,
		// Temperature: 0.7, // 可以考虑从 LLMCharacter 配置中读取
	}

	stream, err := client.CreateChatCompletionStream(ctx, chatAPIReq)
	if err != nil { /* ... 错误处理并返回 ... */ 
	    log.Printf("错误: CreateChatCompletionStream 失败 for model %s: %v", req.Model, err)
	    return "", fmt.Errorf("AI服务暂时不可用 (%s): %w", req.AIName, err)
	}
	defer stream.Close()

	var fullResponseContent strings.Builder
	var streamErr error

	writer.Header().Set("Content-Type", "text/event-stream") // 确保SSE头在这里设置
	writer.Header().Set("Cache-Control", "no-cache")
	writer.Header().Set("Connection", "keep-alive")
	// writer.Flush() // 第一次 flush 应该在有数据时

	initialFlushDone := false // 确保header和第一次数据一起flush

	for {
		response, recvErr := stream.Recv()
		if errors.Is(recvErr, io.EOF) {
			log.Printf("AI '%s' 流式响应结束.", req.AIName)
			break
		}
		if recvErr != nil {
			streamErr = fmt.Errorf("从流中接收响应失败 for AI %s: %w", req.AIName, recvErr)
			log.Println("错误:", streamErr)
			break
		}

		if len(response.Choices) > 0 {
			content := response.Choices[0].Delta.Content
			if content != "" {
				fullResponseContent.WriteString(content)
				
				// 确保 JSON 字符串正确性
				escapedContent := strings.ReplaceAll(content, "\\", "\\\\")
				escapedContent = strings.ReplaceAll(escapedContent, "\"", "\\\"")
				escapedContent = strings.ReplaceAll(escapedContent, "\n", "\\n")

				// ai_name 已包含在 ChatHandler 返回的 SSE 结构中，这里可以简化
				// 或者，如果希望每个delta都带ai_name:
				// data := fmt.Sprintf("data: {\"content\": \"%s\", \"ai_name\": \"%s\"}\n\n", escapedContent, req.AIName)
				data := fmt.Sprintf("data: {\"content\": \"%s\"}\n\n", escapedContent)


				if _, writeErr := writer.Write([]byte(data)); writeErr != nil {
					streamErr = fmt.Errorf("写入SSE数据到客户端失败: %w", writeErr)
					log.Println("错误:", streamErr)
					break
				}
				if flusher, ok := writer.(http.Flusher); ok {
					flusher.Flush()
					initialFlushDone = true
				} else if !initialFlushDone { // 如果不支持flusher，至少在第一次数据后尝试flush header
				    // (Gin通常会自动处理，但显式一点无害)
				    // http.ResponseWriter 本身没有 Flush()，依赖 http.Flusher 类型断言
				    log.Println("警告: ResponseWriter 不支持 Flusher")
				    initialFlushDone = true // 避免重复打印日志
				}
			}
		}
	}

	if streamErr != nil {
		// 即使流出错，也返回已收集到的部分内容，上层可能仍需保存或记录
		return fullResponseContent.String(), streamErr
	}

	finalReply := fullResponseContent.String()
	log.Printf("AI '%s' 生成的完整回复 (ChatService): %.100s...", req.AIName, finalReply)
	return finalReply, nil
}

func (s *chatService) GetChatHistory(userID string) ([]models.ChatMessage, error) {
	// ChatService 当前不直接管理 repo，此方法应由 ChatHandler 调用 repo 实现
	// 或者 NewChatService 时注入 repo 实例
	return nil, errors.New("ChatService.GetChatHistory: repository not injected or not intended for direct use here")
}