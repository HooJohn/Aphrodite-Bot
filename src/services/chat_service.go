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
	"project/repository" // <<< ADDED IMPORT
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

type chatService struct {
	chatRepo repository.ChatRepository // <<< ADDED FIELD
}

func NewChatService(chatRepo repository.ChatRepository) ChatService { // <<< MODIFIED PARAMETER
	return &chatService{chatRepo: chatRepo} // <<< INITIALIZE FIELD
}

func (s *chatService) ProcessMessageStream(
	currentUserMessage models.ChatMessage, // 当前用户的消息对象
	req ChatRequest,
	writer http.ResponseWriter,
	additionalContextHeader string, // 接收此参数
) (string, error) {

	providerKey, modelExists := config.AppConfig.LLMModels[req.Model]
	if !modelExists {
		errMsg := fmt.Sprintf("model '%s' not found in LLMModels configuration", req.Model)
		log.Printf("ERROR: [ChatService] ProcessMessageStream: %s", errMsg)
		return "", errors.New(errMsg) // Or a more specific error type
	}
	providerConfig, providerExists := config.AppConfig.LLMProviders[providerKey]
	if !providerExists {
		errMsg := fmt.Sprintf("provider key '%s' for model '%s' not found in LLMProviders configuration", providerKey, req.Model)
		log.Printf("ERROR: [ChatService] ProcessMessageStream: %s", errMsg)
		return "", errors.New(errMsg)
	}
	apiKey := providerConfig.APIKey
	baseURL := providerConfig.BaseURL
	if apiKey == "" || baseURL == "" {
		errMsg := fmt.Sprintf("API key or BaseURL is empty for provider '%s' (model '%s')", providerKey, req.Model)
		log.Printf("ERROR: [ChatService] ProcessMessageStream: %s", errMsg)
		return "", errors.New(errMsg)
	}

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
		log.Printf("INFO: [ChatService] Building System Prompt for AI '%s': %.100s...", req.AIName, systemPrompt)
		llmMessages = append(llmMessages, openai.ChatCompletionMessage{
			Role: openai.ChatMessageRoleSystem, Content: systemPrompt,
		})
	}

	if additionalContextHeader != "" {
		log.Printf("INFO: [ChatService] Injecting additional context header for AI '%s': %.100s...", req.AIName, additionalContextHeader)
		llmMessages = append(llmMessages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleAssistant, // As AI's received task instruction
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
	if err != nil {
		errMsg := fmt.Sprintf("failed to create chat completion stream for model %s (AI: %s, UserID: %s)", req.Model, req.AIName, req.UserID)
		log.Printf("ERROR: [ChatService] %s: %v", errMsg, err)
		return "", fmt.Errorf("%s: %w", errMsg, err)
	}
	defer stream.Close()

	var fullResponseContent strings.Builder
	var streamErr error // To store errors encountered during streaming

	writer.Header().Set("Content-Type", "text/event-stream") // Ensure SSE header is set here
	writer.Header().Set("Cache-Control", "no-cache")
	writer.Header().Set("Connection", "keep-alive")
	// writer.Flush() // 第一次 flush 应该在有数据时

	initialFlushDone := false // 确保header和第一次数据一起flush

	for {
		response, recvErr := stream.Recv()
		if errors.Is(recvErr, io.EOF) {
			log.Printf("INFO: [ChatService] Stream ended for AI '%s', UserID '%s'.", req.AIName, req.UserID)
			break
		}
		if recvErr != nil {
			streamErr = fmt.Errorf("failed to receive from stream for AI %s (UserID: %s): %w", req.AIName, req.UserID, recvErr)
			log.Printf("ERROR: [ChatService] %v", streamErr)
			break // Critical stream error, stop processing
		}

		if len(response.Choices) > 0 {
			content := response.Choices[0].Delta.Content
			if content != "" {
				fullResponseContent.WriteString(content)
				
				escapedContent := strings.ReplaceAll(content, "\\", "\\\\")
				escapedContent = strings.ReplaceAll(escapedContent, "\"", "\\\"")
				escapedContent = strings.ReplaceAll(escapedContent, "\n", "\\n")
				data := fmt.Sprintf("data: {\"content\": \"%s\"}\n\n", escapedContent)

				if _, writeErr := writer.Write([]byte(data)); writeErr != nil {
					streamErr = fmt.Errorf("failed to write SSE data to client for AI %s (UserID: %s): %w", req.AIName, req.UserID, writeErr)
					log.Printf("ERROR: [ChatService] %v", streamErr)
					break // Client connection likely lost
				}
				if flusher, ok := writer.(http.Flusher); ok {
					flusher.Flush()
					initialFlushDone = true
				} else if !initialFlushDone {
				    log.Printf("WARN: [ChatService] ResponseWriter does not support Flusher for AI '%s', UserID '%s'. SSE might be buffered.", req.AIName, req.UserID)
				    initialFlushDone = true 
				}
			}
		}
	}

	// If streamErr occurred, it's logged above. The function returns the content accumulated so far, and the error.
	// The handler (ChatHandler) should be aware that the stream might have been partial.
	// No specific action for streamErr here, as it's returned.

	finalReply := fullResponseContent.String()
	if streamErr != nil {
		log.Printf("WARN: [ChatService] AI '%s' (UserID: %s) stream completed with error. Partial reply length: %d. Error: %v", req.AIName, req.UserID, len(finalReply), streamErr)
	} else {
		log.Printf("INFO: [ChatService] AI '%s' (UserID: %s) generated full reply. Length: %d. Preview: %.100s...", req.AIName, req.UserID, len(finalReply), finalReply)
	}
	return finalReply, streamErr // Return accumulated content and any stream error
}

func (s *chatService) GetChatHistory(userID string) ([]models.ChatMessage, error) {
	log.Printf("INFO: [ChatService] GetChatHistory called for userID: %s", userID)
	if s.chatRepo == nil {
		log.Printf("ERROR: [ChatService] chatRepo is nil. Ensure NewChatService was called with a valid repository.")
		return nil, errors.New("chat service internal error: repository not initialized")
	}
	messages, err := s.chatRepo.GetMessagesByUserID(userID)
	if err != nil {
		errMsg := fmt.Sprintf("failed to get chat history for userID %s from repository", userID)
		log.Printf("ERROR: [ChatService] %s: %v", errMsg, err)
		return nil, fmt.Errorf("%s: %w", errMsg, err)
	}
	log.Printf("INFO: [ChatService] Successfully retrieved %d messages for userID %s from repository", len(messages), userID)
	return messages, nil
}