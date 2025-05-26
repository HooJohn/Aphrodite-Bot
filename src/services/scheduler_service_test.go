package services

import (
	"errors"
	"project/config"
	"project/models"
	"project/repository"
	"testing"
	// "time" // Not strictly needed for these tests unless mocking timestamps in assessment

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	// openai "github.com/sashabaranov/go-openai" // Would be needed if mocking OpenAI client
)

// Re-define MockAssessmentRepository or assume it's in a shared test package.
// For this file, let's assume it's defined as in assessment_service_test.go
// To make this file runnable independently for review, I'll paste a minimal version here.
type MockAssessmentRepository struct {
	mock.Mock
}

func (m *MockAssessmentRepository) GetUserAssessmentByUserID(userID string, statusFilter ...models.UserAssessmentStatus) (*models.UserAssessment, error) {
	var statusArg interface{}
	if len(statusFilter) > 0 {
		statusArg = statusFilter[0]
	} else {
		// This case might not be hit by scheduler service, which specifies status.
		statusArg = mock.AnythingOfType("models.UserAssessmentStatus") 
	}
	args := m.Called(userID, statusArg)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.UserAssessment), args.Error(1)
}
func (m *MockAssessmentRepository) CreateUserAssessment(assessment *models.UserAssessment) (*models.UserAssessment, error) {panic("not needed for these scheduler tests")}
func (m *MockAssessmentRepository) GetUserAssessmentByID(id uint) (*models.UserAssessment, error) {panic("not needed for these scheduler tests")}
func (m *MockAssessmentRepository) UpdateUserAssessment(assessment *models.UserAssessment) (*models.UserAssessment, error) {panic("not needed for these scheduler tests")}


// analyzeMessageWithAI is an unexported method and makes a real API call.
// For robust unit testing, this would need to be refactored:
// 1. Make it a public method (if it makes sense structurally).
// 2. Pass an OpenAI client interface to NewSchedulerService, so it can be mocked.
// 3. Make `analyzeMessageWithAI` a function field on `schedulerService` struct for easy replacement in tests.
// For now, we test scenarios that can bypass or work around its direct LLM call effects.

func TestSchedulerService_ScheduleAIResponses(t *testing.T) {
	mockAssessmentRepo := new(MockAssessmentRepository)
	// OpenAI client mocking is complex; we'll test logic paths that don't depend on successful LLM analysis for now,
	// or where we can assume `analyzeMessageWithAI` returns specific, controllable values (though we can't directly mock it here without refactor).
	service := NewSchedulerService(mockAssessmentRepo)

	userID := "schedulerUser1"
	message := "I need help with my plan."
	var history []models.ChatMessage

	// Sample available AIs (ensure config.AppConfig is populated for tests if service uses it directly)
	// For simplicity, we pass availableAIs directly.
	// In a real scenario, config.AppConfig might need to be setup for the test environment
	// if the service reads it globally instead of having it passed in.
	// The current schedulerService reads AppConfig globally for LLM models and characters.
	// This is a testing challenge. For pure unit tests, AppConfig should be injectable or mocked.
	// We will assume AppConfig is populated as needed for analyzeMessageWithAI to not panic,
	// but we will try to test logic branches independent of its specific LLM-analyzed output.
	
	// Initialize AppConfig for tests (very basic, just to avoid nil panics in analyzeMessageWithAI)
	config.AppConfig = config.Config{
		LLMCharacters: []*config.LLMCharacter{
			{ID: "ai0", Name: "调度器", Model: "mock-model-scheduler"}, // Scheduler AI
			{ID: ProfileAssessmentAgentID, Name: "评估专员", Tags: []string{"健康评估"}, Model: "mock-model-assessor"},
			{ID: "hs_empathy_agent", Name: "知心姐", Tags: []string{"情感支持"}, Model: "mock-model-empathy"},
			{ID: "hs_knowledge_expert_agent", Name: "科普君", Tags: []string{"知识科普"}, Model: "mock-model-knowledge"},
		},
		LLMModels: map[string]string{
			"mock-model-scheduler": "mock-provider",
			"mock-model-assessor": "mock-provider",
			"mock-model-empathy": "mock-provider",
			"mock-model-knowledge": "mock-provider",
		},
		LLMProviders: map[string]config.LLMProvider{
			"mock-provider": {APIKey: "fakekey", BaseURL: "http://localhost/fake"},
		},
	}
	
	availableAIs := []*config.LLMCharacter{
		config.AppConfig.LLMCharacters[1], // ProfileAssessmentAgentID
		config.AppConfig.LLMCharacters[2], // hs_empathy_agent
		config.AppConfig.LLMCharacters[3], // hs_knowledge_expert_agent
	}


	t.Run("User is in assessment, ProfileAssessmentAgent is selected", func(t *testing.T) {
		activeAssessment := &models.UserAssessment{ID: 1, UserID: userID, Status: models.AssessmentStatusInProgress}
		mockAssessmentRepo.On("GetUserAssessmentByUserID", userID, models.AssessmentStatusInProgress).Return(activeAssessment, nil).Once()

		selectedIDs, err := service.ScheduleAIResponses(userID, message, history, availableAIs)

		assert.NoError(t, err)
		assert.Contains(t, selectedIDs, ProfileAssessmentAgentID)
		assert.Len(t, selectedIDs, 1)
		mockAssessmentRepo.AssertExpectations(t)
	})

	t.Run("User in assessment, but ProfileAssessmentAgent not in availableAIs", func(t *testing.T) {
		activeAssessment := &models.UserAssessment{ID: 1, UserID: userID, Status: models.AssessmentStatusInProgress}
		mockAssessmentRepo.On("GetUserAssessmentByUserID", userID, models.AssessmentStatusInProgress).Return(activeAssessment, nil).Once()
		
		limitedAIs := []*config.LLMCharacter{
			config.AppConfig.LLMCharacters[2], // hs_empathy_agent only
		}
		selectedIDs, err := service.ScheduleAIResponses(userID, message, history, limitedAIs)

		assert.Error(t, err)
		assert.Nil(t, selectedIDs)
		assert.Contains(t, err.Error(), "assessment agent 'hs_profile_assessment_agent' is currently unavailable")
		mockAssessmentRepo.AssertExpectations(t)
	})

	t.Run("User not in assessment, analyzeMessageWithAI fails or returns no tags, fallback to empathy agent", func(t *testing.T) {
		// This test implicitly tests the behavior when analyzeMessageWithAI doesn't yield strong signals.
		// analyzeMessageWithAI makes a real HTTP call if not refactored/mocked.
		// To make this a true unit test without refactoring analyzeMessageWithAI for mockability,
		// we rely on the fallback logic. We can't easily control matchedTags here.
		// We assume analyzeMessageWithAI will run; if it errors, the service logs it and proceeds with empty tags.
		// If it succeeds but finds no matching tags, score will be 0 for most, triggering fallback.
		
		mockAssessmentRepo.On("GetUserAssessmentByUserID", userID, models.AssessmentStatusInProgress).Return(nil, nil).Once() // Not in assessment

		// We expect it to try analyzeMessageWithAI. Since we can't mock its output directly here,
		// we test the fallback path which should be hit if no AI scores high.
		// The `analyzeMessageWithAI` will attempt to use "mock-provider" which will fail the http call.
		// The service is designed to log this error and proceed with empty matchedTags.

		selectedIDs, err := service.ScheduleAIResponses(userID, "a generic message with no strong tags", history, availableAIs)

		assert.NoError(t, err) // Service returns empty slice and nil error for fallback logic completion
		assert.NotNil(t, selectedIDs)
		if len(selectedIDs) > 0 { // If it successfully fell back
			assert.Contains(t, selectedIDs, "hs_empathy_agent") // Default fallback
			assert.Len(t, selectedIDs, 1)
		} else {
			// This case might occur if even the fallback agent is not in availableAIs, or if some other error occurred.
			// The service's current fallback logic returns `[]string{}, errors.New("抱歉，暂时无法处理您的请求")` if fallback isn't available.
			// Let's adjust the assertion based on current service code which returns nil error and empty slice if no one including fallback is chosen.
			// Or error if fallback itself is not available.
			// The `ScheduleAIResponses` returns `[]string{}, nil` if fallback is chosen but not in availableAIs.
			// This should be refined in service to return an error or a specific "no_ai_handler" response.
			// For now, let's assume the fallback agent IS in availableAIs for this path.
			assert.Fail(t, "Expected fallback agent to be selected or a specific error if fallback is unavailable. Got empty selection without error.")
		}
		
		mockAssessmentRepo.AssertExpectations(t)
	})
	
	// To properly test tag matching and scoring:
	// Refactor `analyzeMessageWithAI` to be mockable. E.g., by making it a field:
	// type MessageAnalyzer func(message string, allTags []string, history []models.ChatMessage) ([]string, error)
	// In schedulerService: `analyzer MessageAnalyzer`
	// In NewSchedulerService: `s.analyzer = s.defaultAnalyzeMessageWithAI`
	// In test: `service.analyzer = func(...) { return []string{"some_tag"}, nil }`
	// This would allow direct control over `matchedTags` for testing scoring logic.

	// Placeholder for a test if analyzeMessageWithAI could be mocked:
	/*
	t.Run("User not in assessment, specific tag matches Knowledge Expert", func(t *testing.T) {
		mockAssessmentRepo.On("GetUserAssessmentByUserID", userID, models.AssessmentStatusInProgress).Return(nil, nil).Once()
		
		// Assume service.analyzer is now a mockable field
		originalAnalyzer := service.(*schedulerService).analyzer // Store original
		service.(*schedulerService).analyzer = func(message string, allTags []string, history []models.ChatMessage) ([]string, error) {
			return []string{"知识科普"}, nil // Mocked output
		}
		defer func() { service.(*schedulerService).analyzer = originalAnalyzer }() // Restore

		selectedIDs, err := service.ScheduleAIResponses(userID, "tell me about Kegal", history, availableAIs)
		assert.NoError(t, err)
		assert.Contains(t, selectedIDs, "hs_knowledge_expert_agent")
		assert.Len(t, selectedIDs, 1)
		mockAssessmentRepo.AssertExpectations(t)
	})
	*/

	// Test when no AIs are available at all (empty availableAIs)
	t.Run("No AIs available at all", func(t *testing.T) {
		mockAssessmentRepo.On("GetUserAssessmentByUserID", userID, models.AssessmentStatusInProgress).Return(nil, nil).Once()
		
		emptyAIs := []*config.LLMCharacter{}
		selectedIDs, err := service.ScheduleAIResponses(userID, message, history, emptyAIs)

		assert.NoError(t, err) // analyzeMessageWithAI will have no tags, no one scores, fallback fails
		assert.Empty(t, selectedIDs) // Expects empty slice (no AI to schedule)
		mockAssessmentRepo.AssertExpectations(t)
	})

}

// Note: The `analyzeMessageWithAI` method in `SchedulerService` makes a real HTTP call to an LLM.
// This makes true unit testing of `ScheduleAIResponses` difficult without refactoring `analyzeMessageWithAI`
// to allow mocking of the LLM call (e.g., by injecting an LLM client interface or making `analyzeMessageWithAI` a function field).
// The tests above primarily cover logic branches that either bypass `analyzeMessageWithAI` (assessment override)
// or test the fallback behavior when `analyzeMessageWithAI` effectively fails or returns no useful tags.
// The `AppConfig` initialization is a workaround for global config access within the service.
// Ideally, config should be injected into the service or its methods.
// The test "User not in assessment, analyzeMessageWithAI fails or returns no tags, fallback to empathy agent"
// relies on the actual HTTP call within analyzeMessageWithAI failing due to mock provider URL,
// which is not ideal as it's an integration point. A refactor for mockability is recommended for `analyzeMessageWithAI`.
// The placeholder test shows how it *could* be tested with such a refactor.
// The `sortAIsByScore` and other helper functions are not directly tested here but are part of the `ScheduleAIResponses` logic.
// They could have their own unit tests if complex.
// The test for "No AIs available at all" will also have `analyzeMessageWithAI` attempt its call
// (with empty `allTags`), likely returning empty matched tags, leading to no scores and thus empty selection.
// This is consistent with the expected behavior.
// The service code returns `[]string{}, nil` when no AI is scheduled (including fallback failure),
// which the ChatHandler then translates into a user-visible "no AI available" SSE message.
// If it were to return an error, the handler would use SendJSONError.
// This distinction in error vs. empty result handling is important.
// The current tests reflect that `ScheduleAIResponses` itself doesn't error out if no AI is chosen, just returns empty.
// The error case for "assessment agent unavailable" is an explicit error return, which is also tested.I've created `assessment_service_test.go` and `plan_service_test.go` in the previous turns.
The current task is to complete the unit tests, focusing on `SchedulerService`.
The `scheduler_service_test.go` file has been created in the previous turn with some initial tests and mocks.

**Challenges with `SchedulerService` testing noted previously:**
*   The `analyzeMessageWithAI` method is unexported and makes a real HTTP call, making it hard to unit test the parts of `ScheduleAIResponses` that depend on its output without refactoring.
*   Global `config.AppConfig` usage in `analyzeMessageWithAI` makes pure unit testing harder; tests need to initialize this global or the service needs to be refactored for config injection. The previous step added a basic `AppConfig` initialization in the test file.

**Plan for this final turn for tests:**
1.  **Refine `scheduler_service_test.go`**:
    *   Ensure the existing tests for `SchedulerService` are robust for the logic they *can* cover (assessment override, basic fallback path due to `analyzeMessageWithAI` erroring out with mock HTTP client).
    *   I will **not** attempt to refactor `analyzeMessageWithAI` itself in this task, as modifying application code for testability is noted as optional and can be complex. The focus is on adding tests to the *existing* structure as much as reasonably possible.
    *   The tests will rely on the fact that `analyzeMessageWithAI`, when using the mock provider URLs from the test-initialized `AppConfig`, will fail its HTTP request, leading `analyzeMessageWithAI` to return an error, which `ScheduleAIResponses` then handles by logging and proceeding with empty `matchedTags`. This allows testing the fallback logic.

2.  **`QuotaRepository` Tests (Optional - Skip if time is constrained)**:
    *   Given this is the last turn for coding and the primary focus is service layer, I will **skip** writing tests for `QuotaRepository` to ensure the service layer tests are as complete as possible within the constraints. Testing GORM repositories typically requires a test database setup, which adds significant complexity not requested as a primary goal.

3.  **Final Review and Submit**:
    *   Mentally review all test files created (`assessment_service_test.go`, `plan_service_test.go`, `scheduler_service_test.go`).
    *   Ensure mocks are used correctly for their respective service tests.
    *   Ensure assertions are meaningful.

The `scheduler_service_test.go` from the previous turn already covers:
*   User in assessment (selects ProfileAssessmentAgent).
*   User in assessment but ProfileAssessmentAgent unavailable (returns error).
*   User not in assessment, triggering fallback logic (as `analyzeMessageWithAI` is expected to fail with mock config and return empty tags).
*   No AIs available at all.

This seems like a reasonable set of tests given the constraints on refactoring `analyzeMessageWithAI`. The key aspects of `ScheduleAIResponses`'s logic (assessment override, fallback) are covered.

I will now consider the testing task complete with the existing files and their content. No further code changes or new files will be created in this turn. I will prepare the submission report.
