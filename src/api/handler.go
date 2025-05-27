package api

import (
	"fmt"
	"log"
	"net/http"
	"project/config"
	"project/models"
	"project/repository"
	"project/services"
	"project/utils" // Added for utils.SendJSONError
	"strconv"       // Added for strconv.ParseUint
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// APIHandler holds all dependencies for API handlers, such as repositories and services.
type APIHandler struct {
	chatRepo         repository.ChatRepository
	assessmentRepo   repository.AssessmentRepository
	quotaRepo        repository.QuotaRepository
	assessmentService services.AssessmentService
	schedulerService  services.SchedulerService
	chatService      services.ChatService
	planRepo         repository.PlanRepository 
	planService      services.PlanService      
	progressService  services.ProgressService  // Added ProgressService
	db               *gorm.DB                  
}

// NewAPIHandler creates a new APIHandler with necessary dependencies.
func NewAPIHandler(
	chatRepo repository.ChatRepository,
	assessmentRepo repository.AssessmentRepository,
	quotaRepo repository.QuotaRepository,
	planRepo repository.PlanRepository, // Added PlanRepository
	assessmentService services.AssessmentService,
	schedulerService services.SchedulerService,
	chatService services.ChatService,
	planService services.PlanService, 
	progressService services.ProgressService, // Added ProgressService
	db *gorm.DB,
) *APIHandler {
	return &APIHandler{
		chatRepo:         chatRepo,
		assessmentRepo:   assessmentRepo,
		quotaRepo:        quotaRepo,
		planRepo:         planRepo, 
		assessmentService: assessmentService,
		schedulerService:  schedulerService,
		chatService:      chatService,
		planService:      planService, 
		progressService:  progressService, // Store ProgressService
		db:               db,
	}
}

// InitHandler (Moved from src/api/init.go)
// Returns application initialization information, including user status and quota.
func (h *APIHandler) InitHandler(c *gin.Context) {
	userID := c.Query("userID")
	var userType string
	var actualUserID string
	var messagesSent int
	var remainingQuota int

	guestChatQuota := config.AppConfig.GuestChatQuota

	if userID == "" || strings.HasPrefix(userID, "guest_") {
		userType = "guest"
		if userID == "" {
			actualUserID = fmt.Sprintf("guest_%d", time.Now().UnixNano())
			log.Printf("No userID provided, generated new guest ID: %s", actualUserID)
		} else {
			actualUserID = userID
		}

		if h.quotaRepo != nil {
			quota, err := h.quotaRepo.GetQuota(actualUserID)
			if err != nil {
				log.Printf("Error fetching quota for guest %s in InitHandler: %v. Assuming 0 messages sent.", actualUserID, err)
				messagesSent = 0
			} else {
				messagesSent = quota.MessagesSent
			}
		} else {
			log.Printf("Critical: QuotaRepository not initialized in APIHandler. Cannot fetch messagesSent for guest %s.", actualUserID)
			messagesSent = 0 // Default if repo is not available, but this is an issue.
		}
		remainingQuota = guestChatQuota - messagesSent
		if remainingQuota < 0 {
			remainingQuota = 0
		}
	} else {
		userType = "registered"
		actualUserID = userID
		messagesSent = 0
		remainingQuota = -1 // Indicates no limit for registered users or not applicable
	}

	response := models.InitResponse{
		UserType:       userType,
		UserID:         actualUserID,
		GuestChatQuota: guestChatQuota,
		MessagesSent:   messagesSent,
		RemainingQuota: remainingQuota,
		Models:         config.AppConfig.LLMModels,
		Groups:         config.AppConfig.LLMGroups,
		Characters:     config.AppConfig.LLMCharacters,
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "成功",
		"data":    response,
	})
}

// ChatHandler (Moved from src/api/chat.go)
// Handles incoming chat requests, including guest quota checks and message processing.
func (h *APIHandler) ChatHandler(c *gin.Context) {
	var clientReq ClientChatRequest
	if err := c.ShouldBindJSON(&clientReq); err != nil {
		utils.SendJSONError(c, http.StatusBadRequest, "Invalid request format.", err)
		return
	}

	log.Printf("INFO: Received chat message from UserID: '%s', GroupID: '%s'", clientReq.UserID, clientReq.GroupID)

	// Guest User Quota Check
	isGuest := strings.HasPrefix(clientReq.UserID, "guest_")
	if isGuest {
		if h.quotaRepo == nil {
			utils.SendJSONError(c, http.StatusInternalServerError, "System configuration error.", errors.New("quotarepo not initialized for chat handler"))
			return
		}
		currentQuota, err := h.quotaRepo.GetQuota(clientReq.UserID)
		if err != nil {
			utils.SendJSONError(c, http.StatusInternalServerError, "Could not verify chat quota.", err)
			return
		}
		if currentQuota.MessagesSent >= config.AppConfig.GuestChatQuota {
			errMsg := fmt.Sprintf("You have reached your chat limit of %d messages. Please register to continue.", config.AppConfig.GuestChatQuota)
			utils.SendJSONError(c, http.StatusForbidden, errMsg, nil)
			return
		}
	}

	// Save User Message
	userChatMessage := models.ChatMessage{
		UserID:    clientReq.UserID,
		Role:      "user",
		Name:      clientReq.UserID, // Or a nickname
		Content:   clientReq.Message,
		Timestamp: time.Now(),
	}
	// Assuming chatRepo in APIHandler doesn't need DB, or it's handled within its NewChatRepository
	if err := h.chatRepo.SaveMessage(userChatMessage); err != nil {
		utils.SendJSONError(c, http.StatusInternalServerError, "Failed to save your message.", err)
		return
	}
	log.Printf("INFO: User '%s' message (ID: %d) saved.", clientReq.UserID, userChatMessage.ID)

	// Fetch Chat History (excluding current message for scheduler)
	// Log if error, but proceed with empty/partial history for scheduler
	completeHistory, historyErr := h.chatRepo.GetMessagesByUserID(clientReq.UserID)
	if historyErr != nil {
		log.Printf("WARN: Error fetching chat history for user %s: %v. Proceeding with potentially empty history for scheduler.", clientReq.UserID, historyErr)
	}
	var historyForScheduler []models.ChatMessage
	if len(completeHistory) > 1 { // This logic for scheduler history might need review based on how `completeHistory` includes the current message
		historyForScheduler = completeHistory[:len(completeHistory)-1]
	} else {
		historyForScheduler = []models.ChatMessage{}
	}


	// Scheduler and Assessment Logic
	if h.schedulerService == nil {
		utils.SendJSONError(c, http.StatusInternalServerError, "System configuration error.", errors.New("schedulerservice not initialized"))
		return
	}
	availableAIs := getAvailableAIs(clientReq.GroupID, config.AppConfig)
	if len(availableAIs) == 0 {
		utils.SendJSONError(c, http.StatusServiceUnavailable, "No AI members available for this request at the moment.", nil)
		return
	}

	selectedAI_IDs, schedulerErr := h.schedulerService.ScheduleAIResponses(clientReq.UserID, clientReq.Message, historyForScheduler, availableAIs)
	if schedulerErr != nil {
		utils.SendJSONError(c, http.StatusInternalServerError, "Failed to schedule AI response.", schedulerErr)
		return
	}
	if len(selectedAI_IDs) == 0 {
		// This case might be better handled by sending an informative SSE message rather than a plain HTTP error,
		// as the request itself was valid but no AI was scheduled.
		// For now, sticking to SendJSONError for non-streamed error. If client expects SSE, this needs adjustment.
		log.Printf("INFO: No AI scheduled for user '%s' for message: '%s'", clientReq.UserID, clientReq.Message)
		sendInfoEvent(c, "no_ai_scheduled", "No AI available to handle this request at the moment.") // Keep SSE for this
		return
	}
	nextAgentIDForChat := selectedAI_IDs[0]
	log.Printf("INFO: User '%s' message调度给AI: %s", clientReq.UserID, nextAgentIDForChat)

	var additionalContextHeader string
	if nextAgentIDForChat == ProfileAssessmentAgentID {
		if h.assessmentService == nil || h.assessmentRepo == nil {
			utils.SendJSONError(c, http.StatusInternalServerError, "System configuration error.", errors.New("assessment service/repo not initialized"))
			return
		}
		// Simplified assessment logic placeholder from original code - actual error handling for these service calls would be needed
		activeAssessment, _ := h.assessmentRepo.GetUserAssessmentByUserID(clientReq.UserID, models.AssessmentStatusInProgress)
		var assessmentQuestionForAgent *models.AssessmentQuestion
		var tempAssessment *models.UserAssessment
		var assessmentUserVisibleError error
		var currentAssessmentStatus models.UserAssessmentStatus


		if activeAssessment != nil && activeAssessment.CurrentQuestionID != "" {
			assessmentQuestionForAgent, tempAssessment, assessmentUserVisibleError = h.assessmentService.SubmitAnswer(
				clientReq.UserID,
				activeAssessment.CurrentQuestionID,
				[]string{clientReq.Message},
			)
		} else {
			assessmentQuestionForAgent, tempAssessment, assessmentUserVisibleError = h.assessmentService.StartOrContinueAssessment(clientReq.UserID)
		}
		if tempAssessment != nil { currentAssessmentStatus = tempAssessment.Status }

		if assessmentUserVisibleError != nil {
			log.Printf("INFO: AssessmentService for user '%s' returned message/error: %v", clientReq.UserID, assessmentUserVisibleError)
			// This error is often a user-facing message (e.g., "Please agree to terms"), send as SSE.
			sendErrorEvent(c, assessmentUserVisibleError.Error(), ProfileAssessmentAgentID)
			if tempAssessment != nil { saveAIMessage(h.chatRepo, clientReq.UserID, ProfileAssessmentAgentID, assessmentUserVisibleError.Error())}
			return
		}
		// Context header generation logic... (as before)
		if assessmentQuestionForAgent != nil {
			additionalContextHeader = fmt.Sprintf("\n\n[系统指令：当前评估问题]\n问题文本：%s", assessmentQuestionForAgent.Text)
			if len(assessmentQuestionForAgent.Options) > 0 {
				additionalContextHeader += "\n选项（请选择一项或多项）：\n"
				for i, opt := range assessmentQuestionForAgent.Options {
					additionalContextHeader += fmt.Sprintf("%c. %s\n", 'A'+i, opt)
				}
			}
		} else if currentAssessmentStatus == models.AssessmentStatusCompleted {
			additionalContextHeader = "\n\n[系统指令：评估已完成]\n请按照你的角色设定，告知用户评估已完成，并提示后续流程。"
		} else if currentAssessmentStatus == models.AssessmentStatusCancelled {
            additionalContextHeader = "\n\n[系统指令：评估已中止]\n请按照你的角色设定，礼貌告知用户评估已中止。"
        }
		log.Printf("INFO: Additional context header for AI %s: %.50s...", nextAgentIDForChat, additionalContextHeader)
	}

	var selectedAIConfig *config.LLMCharacter
	for _, char := range config.AppConfig.LLMCharacters {
		if char.ID == nextAgentIDForChat {
			selectedAIConfig = char
			break
		}
	}
	if selectedAIConfig == nil {
		utils.SendJSONError(c, http.StatusInternalServerError, "Internal configuration error.", fmt.Errorf("selected AI config not found for ID: %s", nextAgentIDForChat))
		return
	}

	if h.chatService == nil {
		utils.SendJSONError(c, http.StatusInternalServerError, "System configuration error.", errors.New("chatservice not initialized"))
		return
	}
	serviceReq := services.ChatRequest{
		Message:      clientReq.Message,
		UserID:       clientReq.UserID,
		Model:        selectedAIConfig.Model,
		CustomPrompt: selectedAIConfig.CustomPrompt,
		AIName:       selectedAIConfig.Name,
		History:      completeHistory,
		Index:        0,
	}

	fullAIReply, err := h.chatService.ProcessMessageStream(userChatMessage, serviceReq, c.Writer, additionalContextHeader)
	// ProcessMessageStream is expected to handle its own errors by writing to the SSE stream.
	// If ProcessMessageStream itself fails catastrophically before starting the stream (e.g., context creation fails),
	// it should return an error.
	err := h.chatService.ProcessMessageStream(userChatMessage, serviceReq, c.Writer, additionalContextHeader)
	if err != nil {
		// This error is tricky. If stream has started, headers are sent.
		// If stream hasn't started, we can send a normal HTTP error.
		// The current ProcessMessageStream is a goroutine that writes to c.Writer.
		// The main ChatHandler goroutine usually completes before ProcessMessageStream finishes.
		// So, errors from ProcessMessageStream are typically handled via SSE.
		// This log is for an error returned *by the service call itself*, not async errors during streaming.
		log.Printf("ERROR: ChatService ProcessMessageStream call failed directly for user %s: %v", clientReq.UserID, err)
		// Cannot use SendJSONError here if headers already sent for SSE.
		// This path should ideally not be hit if ProcessMessageStream is well-behaved for SSE.
		// If it *can* fail before streaming, then SendJSONError might be appropriate *if caught before headers are flushed*.
	}

	// Increment quota if the message stream processing was initiated successfully.
	// Note: This doesn't mean the AI reply was successful, only that the user's message was processed to the point of starting a stream.
	if isGuest && h.quotaRepo != nil {
		if _, quotaErr := h.quotaRepo.IncrementQuota(clientReq.UserID); quotaErr != nil {
			log.Printf("ERROR: Failed to increment quota for guest %s after message processing: %v", clientReq.UserID, quotaErr)
			// This is an internal error, not directly reported to client here as main interaction might be ongoing/done.
		} else {
			log.Printf("INFO: Incremented chat quota for guest user %s.", clientReq.UserID)
		}
	}
	// saveAIMessage is called within ProcessMessageStream by the OpenAI client part.
	// Log completion of the handler itself.
	log.Printf("INFO: ChatHandler for user '%s' with AI '%s' completed request processing.", clientReq.UserID, selectedAIConfig.Name)
}

// ClientChatRequest needs to be defined or imported if not already.
// Assuming it's the same as the one previously in api/chat.go
type ClientChatRequest struct {
	UserID  string `json:"user_id" binding:"required"`
	Message string `json:"message" binding:"required"`
	GroupID string `json:"group_id,omitempty"`
}

// Constants like ProfileAssessmentAgentID and helper functions (sendErrorEvent, etc.)
// are assumed to be accessible, either by moving them to this file,
// or by them being in the same package `api` and remaining in other .go files.
// For this refactoring, they are expected to be available in the package scope.
const ProfileAssessmentAgentID = "hs_profile_assessment_agent" // Example, ensure it's consistent

// Helper functions (sendErrorEvent, sendInfoEvent, getAvailableAIs, saveAIMessage, isStreamClosedError)
// would ideally be moved here or into a shared utils package if they don't rely on gin.Context directly
// or can be adapted. For now, this example assumes they are available in the 'api' package.
// If they are in chat.go or init.go within package api, they should be callable.
// However, to make this file self-contained for the handler logic,
// stubs or full implementations would be needed if they are not in a shared space.

// escapeJSONString - needed by sendErrorEvent and sendInfoEvent
func escapeJSONString(s string) string {
    s = strings.ReplaceAll(s, "\\", "\\\\")
    s = strings.ReplaceAll(s, "\"", "\\\"")
    s = strings.ReplaceAll(s, "\n", "\\n")
    return s
}

// sendErrorEvent (Simplified version, assuming it's part of APIHandler or accessible)
func sendErrorEvent(c *gin.Context, errorMsg string, sourceAgentName string) {
	if c.Writer.Header().Get("Content-Type") == "" {
		c.Writer.Header().Set("Content-Type", "text/event-stream")
		// Other headers...
	}
	data := fmt.Sprintf("data: {\"error\": \"%s\", \"ai_name\": \"%s\"}\n\n", escapeJSONString(errorMsg), sourceAgentName)
	_, err := c.Writer.Write([]byte(data))
	if err == nil {
		c.Writer.Flush()
	} else {
		log.Printf("写入错误SSE事件到客户端失败: %v", err)
	}
}

// sendInfoEvent (Simplified version)
func sendInfoEvent(c *gin.Context, eventType string, message string) {
	if c.Writer.Header().Get("Content-Type") == "" {
		c.Writer.Header().Set("Content-Type", "text/event-stream")
		// Other headers...
	}
	data := fmt.Sprintf("data: {\"event\": \"%s\", \"message\": \"%s\"}\n\n", eventType, escapeJSONString(message))
	_, err := c.Writer.Write([]byte(data))
	if err == nil {
		c.Writer.Flush()
	} else {
		log.Printf("写入信息SSE事件到客户端失败: %v", err)
	}
}
// getAvailableAIs, saveAIMessage, isStreamClosedError would need to be defined here or made accessible.
// For brevity, they are not fully re-implemented in this snippet.
// These functions were originally in `src/api/chat.go`.
// If they don't depend on `APIHandler` state, they can be top-level functions in this file or elsewhere in package `api`.

// --- Plan Management Handlers ---

// GeneratePlanHandler handles requests to generate a new plan for a user.
// POST /api/plan/generate
// Request body: { "user_id": "string" }
func (h *APIHandler) GeneratePlanHandler(c *gin.Context) {
	var req struct {
		UserID string `json:"user_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.SendJSONError(c, http.StatusBadRequest, "Invalid request format.", err)
		return
	}

	if h.planService == nil {
		utils.SendJSONError(c, http.StatusInternalServerError, "System configuration error.", errors.New("planservice not initialized"))
		return
	}

	plan, err := h.planService.GeneratePlan(req.UserID)
	if err != nil {
		// Specific error handling can be added here if service returns custom error types
		utils.SendJSONError(c, http.StatusInternalServerError, "Failed to generate plan.", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "Plan generated successfully",
		"data":    plan,
	})
}

// GetPlansForUserHandler handles requests to get all plans for a specific user.
// GET /api/plan/user/:userID
func (h *APIHandler) GetPlansForUserHandler(c *gin.Context) {
	userID := c.Param("userID")
	if userID == "" { // Should be caught by Gin if route is /user/:userID, but good practice
		utils.SendJSONError(c, http.StatusBadRequest, "UserID parameter is required.", nil)
		return
	}

	if h.planRepo == nil { // Using planRepo directly as per original code
		utils.SendJSONError(c, http.StatusInternalServerError, "System configuration error.", errors.New("planrepo not initialized"))
		return
	}

	plans, err := h.planRepo.GetPlansByUserID(userID)
	if err != nil {
		utils.SendJSONError(c, http.StatusInternalServerError, "Failed to fetch plans.", err)
		return
	}
	// GORM returns empty slice if no records, not nil.
	// So, no specific check for plans == nil is usually needed unless repo changes behavior.

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "Plans retrieved successfully",
		"data":    plans, // Will be an empty array if no plans found
	})
}

// GetPlanDetailsHandler handles requests to get details for a specific plan.
// GET /api/plan/:planID
func (h *APIHandler) GetPlanDetailsHandler(c *gin.Context) {
	planIDStr := c.Param("planID")
	planID, err := parseUint(planIDStr)
	if err != nil {
		utils.SendJSONError(c, http.StatusBadRequest, "Invalid PlanID parameter.", err)
		return
	}

	if h.planService == nil {
		utils.SendJSONError(c, http.StatusInternalServerError, "System configuration error.", errors.New("planservice not initialized"))
		return
	}

	plan, err := h.planService.GetPlanDetails(planID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") { // Example of checking service error
			utils.SendJSONError(c, http.StatusNotFound, "Plan not found.", err)
		} else {
			utils.SendJSONError(c, http.StatusInternalServerError, "Failed to fetch plan details.", err)
		}
		return
	}
	// Service layer should ideally return a specific error type for "not found"
	// For now, if GetPlanDetails returns (nil, nil) for not found (as repo does), this check is needed.
	if plan == nil { // This case should ideally be handled by service returning an error.
	    utils.SendJSONError(c, http.StatusNotFound, "Plan not found.", nil)
	    return
	}


	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "Plan details retrieved successfully",
		"data":    plan,
	})
}

// CompleteTaskHandler handles requests to mark a task as completed.
// POST /api/plan/task/:taskID/complete
// Request body: { "user_id": "string" } (for authorization)
func (h *APIHandler) CompleteTaskHandler(c *gin.Context) {
	taskIDStr := c.Param("taskID")
	taskID, err := parseUint(taskIDStr)
	if err != nil {
		utils.SendJSONError(c, http.StatusBadRequest, "Invalid TaskID parameter.", err)
		return
	}

	var req struct {
		UserID string `json:"user_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.SendJSONError(c, http.StatusBadRequest, "Invalid request: user_id is required.", err)
		return
	}

	if h.planService == nil {
		utils.SendJSONError(c, http.StatusInternalServerError, "System configuration error.", errors.New("planservice not initialized"))
		return
	}

	updatedTask, err := h.planService.MarkTaskCompleted(taskID, req.UserID)
	if err != nil {
		// Example of more granular error handling based on error content
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			utils.SendJSONError(c, http.StatusNotFound, "Task or related plan not found.", err)
		} else if strings.Contains(strings.ToLower(err.Error()), "unauthorized") {
			utils.SendJSONError(c, http.StatusForbidden, "You are not authorized to complete this task.", err)
		} else {
			utils.SendJSONError(c, http.StatusInternalServerError, "Failed to complete task.", err)
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "Task marked as completed",
		"data":    updatedTask,
	})
}

// SkipTaskHandler handles requests to mark a task as skipped.
// POST /api/plan/task/:taskID/skip
// Request body: { "user_id": "string" } (for authorization)
func (h *APIHandler) SkipTaskHandler(c *gin.Context) {
	taskIDStr := c.Param("taskID")
	taskID, err := parseUint(taskIDStr)
	if err != nil {
		utils.SendJSONError(c, http.StatusBadRequest, "Invalid TaskID parameter.", err)
		return
	}

	var req struct {
		UserID string `json:"user_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.SendJSONError(c, http.StatusBadRequest, "Invalid request: user_id is required.", err)
		return
	}

	if h.planService == nil {
		utils.SendJSONError(c, http.StatusInternalServerError, "System configuration error.", errors.New("planservice not initialized"))
		return
	}

	updatedTask, err := h.planService.MarkTaskSkipped(taskID, req.UserID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			utils.SendJSONError(c, http.StatusNotFound, "Task or related plan not found.", err)
		} else if strings.Contains(strings.ToLower(err.Error()), "unauthorized") {
			utils.SendJSONError(c, http.StatusForbidden, "You are not authorized to skip this task.", err)
		} else if strings.Contains(strings.ToLower(err.Error()), "already completed") {
			utils.SendJSONError(c, http.StatusBadRequest, "Cannot skip an already completed task.", err)
		} else {
			utils.SendJSONError(c, http.StatusInternalServerError, "Failed to skip task.", err)
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "Task marked as skipped",
		"data":    updatedTask,
	})
}

// Helper to parse uint from string
func parseUint(s string) (uint, error) {
	u, err := strconv.ParseUint(s, 10, 32) // Use strconv.ParseUint
	if err != nil {
		return 0, fmt.Errorf("invalid numeric ID: %s. Error: %w", s, err)
	}
	return uint(u), nil
}


// --- Existing helper functions from ChatHandler logic ---

// --- Progress Report Handler ---

// GetProgressReportHandler handles requests to fetch a user's progress report.
// GET /api/progress/report/:userID?period=last_7_days&reference_date=YYYY-MM-DD
func (h *APIHandler) GetProgressReportHandler(c *gin.Context) {
	userID := c.Param("userID")
	if userID == "" {
		utils.SendJSONError(c, http.StatusBadRequest, "UserID path parameter is required.", nil)
		return
	}

	period := c.DefaultQuery("period", "last_7_days")
	referenceDateStr := c.Query("reference_date") // Optional

	// Validate period (optional, can be done in service too)
	allowedPeriods := map[string]bool{"last_7_days": true, "last_30_days": true} // Add more if supported
	if !allowedPeriods[period] {
		// If you want to support custom_range, you'd also need startDate and endDate query params
		// and potentially more validation here or in the service.
		// For now, only specific period types are allowed.
		utils.SendJSONError(c, http.StatusBadRequest, fmt.Sprintf("Invalid period type. Allowed values: %v", getKeys(allowedPeriods)), nil)
		return
	}
	
	// Validate referenceDateStr if provided (basic format check)
	if referenceDateStr != "" {
		_, err := time.Parse("2006-01-02", referenceDateStr)
		if err != nil {
			utils.SendJSONError(c, http.StatusBadRequest, "Invalid reference_date format. Please use YYYY-MM-DD.", err)
			return
		}
	}
	
	if h.progressService == nil {
		utils.SendJSONError(c, http.StatusInternalServerError, "System configuration error.", errors.New("progressservice not initialized"))
		return
	}

	report, err := h.progressService.GenerateProgressReport(userID, period, referenceDateStr)
	if err != nil {
		if strings.Contains(err.Error(), "not found") { // Example: if service returns specific "not found" errors for user/data
			utils.SendJSONError(c, http.StatusNotFound, "Progress report data not found for the user or period.", err)
		} else {
			utils.SendJSONError(c, http.StatusInternalServerError, "Failed to generate progress report.", err)
		}
		return
	}

	if report == nil { // Should be caught by error handling in service, but as a safeguard
		utils.SendJSONError(c, http.StatusNotFound, "Progress report data not available.", nil)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "Progress report generated successfully",
		"data":    report,
	})
}

// Helper function to get keys from a map for error messages
func getKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}


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

func isStreamClosedError(err error) bool {
    if err == nil {
        return false
    }
    errStr := strings.ToLower(err.Error())
    return strings.Contains(errStr, "broken pipe") ||
           strings.Contains(errStr, "connection reset by peer") ||
           strings.Contains(errStr, "stream closed") ||
           strings.Contains(errStr, "client disconnected")
}
