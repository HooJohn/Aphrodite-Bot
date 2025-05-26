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

// AssessmentService defines the interface for assessment-related operations.
type AssessmentService interface {
	StartOrContinueAssessment(userID string) (question *models.AssessmentQuestion, assessment *models.UserAssessment, userVisibleError error)
	SubmitAnswer(userID string, questionID string, answerValues []string) (question *models.AssessmentQuestion, assessment *models.UserAssessment, userVisibleError error)
	GetAssessmentResult(userID string) (*models.UserAssessment, error)
}

// assessmentService implements the AssessmentService interface.
type assessmentService struct {
	repo          repository.AssessmentRepository
	questions     []models.AssessmentQuestion          // Ordered list of assessment questions
	questionsByID map[string]*models.AssessmentQuestion // Quick lookup for questions by ID
}

// NewAssessmentService creates a new instance of AssessmentService.
func NewAssessmentService(repo repository.AssessmentRepository) AssessmentService {
	definedQuestions := getDefaultAssessmentQuestions() // Get question definitions from helper

	// Ensure questions are sorted by their Order field
	sort.Slice(definedQuestions, func(i, j int) bool {
		return definedQuestions[i].Order < definedQuestions[j].Order
	})

	questionsMap := make(map[string]*models.AssessmentQuestion)
	// Create a stable copy of questions to store pointers to, for questionsByID map.
	questionStore := make([]models.AssessmentQuestion, len(definedQuestions))
	copy(questionStore, definedQuestions)

	for i := range questionStore {
		questionsMap[questionStore[i].ID] = &questionStore[i]
	}
	
	return &assessmentService{
		repo:          repo,
		questions:     definedQuestions, 
		questionsByID: questionsMap,     
	}
}

// getDefaultAssessmentQuestions defines the set of questions for the assessment.
// Note: These are currently hardcoded. In a more dynamic system, they might come from a DB or config file.
func getDefaultAssessmentQuestions() []models.AssessmentQuestion {
	// Comments for each question detailing its purpose and options.
	return []models.AssessmentQuestion{
		// Welcome and consent to start assessment
		{ID: "q_welcome", Order: 0, Text: "Hello! To provide you with more personalized services, we need to understand some of your basic information. The whole process will take about 5-10 minutes, and your data will be kept strictly confidential. Are you ready to start?", QuestionType: models.QuestionTypeConfirmation, Options: []string{"Yes, I'm ready", "No, next time"}, IsRequired: true},
		// Demographics
		{ID: "q_age_group", Order: 1, Text: "What is your age group?", QuestionType: models.QuestionTypeSingleChoice, Options: []string{"18-24 years", "25-34 years", "35-44 years", "45-54 years", "55 years and above"}, IsRequired: true},
		// Health History
		{ID: "q_chronic_diseases", Order: 2, Text: "Do you have any chronic diseases diagnosed by a doctor (e.g., hypertension, diabetes, heart disease)? If yes, please briefly explain. If no, you can write 'None'.", QuestionType: models.QuestionTypeOpenText, IsRequired: true},
		{ID: "q_medications", Order: 3, Text: "Are you currently taking any long-term prescription medications? If yes, please briefly state the drug name or type. If no, you can write 'None'.", QuestionType: models.QuestionTypeOpenText, IsRequired: true},
		// Lifestyle
		{ID: "q_exercise_habits", Order: 4, Text: "How often do you currently exercise?", QuestionType: models.QuestionTypeSingleChoice, Options: []string{"Almost never", "1-2 times a week", "3-4 times a week", "5 times a week or more"}, IsRequired: true},
		{ID: "q_sleep_quality", Order: 5, Text: "How is your usual sleep quality?", QuestionType: models.QuestionTypeSingleChoice, Options: []string{"Very good, can ensure 7-8 hours and feel energetic", "Average, occasionally insufficient sleep or poor quality", "Poor, often suffer from insomnia or poor sleep quality"}, IsRequired: true},
		{ID: "q_stress_level", Order: 6, Text: "How would you rate your current overall stress level? (1 for extremely low, 5 for extremely high)", QuestionType: models.QuestionTypeSingleChoice, Options: []string{"1 (Extremely Low)", "2 (Low)", "3 (Moderate)", "4 (High)", "5 (Extremely High)"}, IsRequired: true},
		// Sexual Health Specifics
		{ID: "q_sex_frequency_satisfaction", Order: 7, Text: "Are you satisfied with your current frequency of sexual activity?", QuestionType: models.QuestionTypeSingleChoice, Options: []string{"Very satisfied", "Somewhat satisfied", "Neutral", "Somewhat dissatisfied", "Very dissatisfied", "Currently no sexual activity"}, IsRequired: true},
		{ID: "q_sex_ability_concerns", Order: 8, Text: "In terms of sexual ability (e.g., erection, stamina, libido), what are your main concerns or worries at present? (Multiple choice, select 'No significant concerns' if none)", QuestionType: models.QuestionTypeMultiChoice, Options: []string{"Difficulty achieving or maintaining an erection", "Premature ejaculation or insufficient stamina", "Low libido", "Difficulty reaching orgasm", "Pain during intercourse", "Anxiety about performance", "Lack of sexual knowledge or skills", "No significant concerns"}, IsRequired: true},
		// Goals
		{ID: "q_main_goals", Order: 9, Text: "What are the core goals you most hope to achieve with our AI partners? (Multiple choice)", QuestionType: models.QuestionTypeMultiChoice, Options: []string{"Gain scientific sexual health knowledge", "Improve sexual ability (e.g., stamina, hardness)", "Improve sexual communication with partner", "Adjust unhealthy sexual habits", "Relieve sexual anxiety or stress", "Develop a personalized exercise plan", "Understand and try healthy sexual behavior patterns"}, IsRequired: true},
		// Final Consent
		{ID: "q_privacy_consent", Order: 10, Text: "We reiterate that all your answers will be kept strictly confidential and used solely to provide you with personalized health advice and plans. Do you agree to our processing of this information?", QuestionType: models.QuestionTypeConfirmation, Options: []string{"I agree and continue", "I do not agree"}, IsRequired: true},
	}
}

// StartOrContinueAssessment finds an in-progress assessment for the user or starts a new one.
// It returns the next question to be asked, the current state of the assessment, and any user-visible error.
func (s *assessmentService) StartOrContinueAssessment(userID string) (*models.AssessmentQuestion, *models.UserAssessment, error) {
	// Attempt to retrieve an existing in-progress assessment for the user.
	assessment, err := s.repo.GetUserAssessmentByUserID(userID, models.AssessmentStatusInProgress)
	if err != nil { 
		errMsg := fmt.Sprintf("failed to get in-progress assessment for userID %s", userID)
		log.Printf("ERROR: [AssessmentService] %s: %v", errMsg, err)
		return nil, nil, fmt.Errorf("%s: %w", errMsg, err) 
	}
	
	if assessment == nil { // No in-progress assessment found, create a new one.
		log.Printf("INFO: [AssessmentService] No in-progress assessment found for userID '%s', creating a new one.", userID)
		newAssessment := &models.UserAssessment{
			UserID:    userID,
			Status:    models.AssessmentStatusInProgress,
			StartedAt: time.Now(),
			Answers:   make([]models.UserAnswer, 0),
		}
		assessment, err = s.repo.CreateUserAssessment(newAssessment)
		if err != nil {
			errMsg := fmt.Sprintf("failed to create new assessment for userID %s", userID)
			log.Printf("ERROR: [AssessmentService] %s: %v", errMsg, err)
			return nil, nil, fmt.Errorf("%s: %w", errMsg, err)
		}
		log.Printf("INFO: [AssessmentService] Successfully created new assessment ID %d for userID '%s'", assessment.ID, userID)
	} else {
		log.Printf("INFO: [AssessmentService] User '%s' continues in-progress assessment ID %d, current question ID: '%s'", userID, assessment.ID, assessment.CurrentQuestionID)
	}

	var nextQuestionToShow *models.AssessmentQuestion

	if assessment.Status == models.AssessmentStatusCompleted || assessment.Status == models.AssessmentStatusCancelled {
		log.Printf("INFO: [AssessmentService] Assessment ID %d for userID '%s' has status '%s'. No further questions.", userID, assessment.ID, assessment.Status)
		return nil, assessment, nil 
	}

	if assessment.CurrentQuestionID == "" { 
		if len(s.questions) > 0 {
			nextQuestionToShow = &s.questions[0] 
			// CurrentQuestionID will be set on the assessment object before saving below.
		} else {
			log.Printf("ERROR: [AssessmentService] Assessment question list is empty! Cannot determine first question for assessment ID %d.", assessment.ID)
			assessment.Status = models.AssessmentStatusCompleted 
			if _, updateErr := s.repo.UpdateUserAssessment(assessment); updateErr != nil {
				log.Printf("ERROR: [AssessmentService] Failed to update assessment status to completed for assessmentID %d (empty question list): %v", assessment.ID, updateErr)
			}
			return nil, assessment, errors.New("internal system error: assessment questionnaire is not configured")
		}
	} else { 
		currentQFromDef, exists := s.questionsByID[assessment.CurrentQuestionID]
		if !exists {
			log.Printf("WARN: [AssessmentService] CurrentQuestionID '%s' for assessment %d (userID '%s') not found in definitions! Questionnaire may have changed. Attempting to restart.", assessment.CurrentQuestionID, assessment.ID, userID)
			if len(s.questions) > 0 {
				nextQuestionToShow = &s.questions[0]
				// CurrentQuestionID will be set on assessment object before saving.
			} else { 
				log.Printf("ERROR: [AssessmentService] Assessment question list is empty when trying to handle missing CurrentQuestionID for assessment %d!", assessment.ID)
				return nil, assessment, errors.New("internal system error: assessment questionnaire is not configured")
			}
		} else {
			answeredCurrent := false
			for _, ans := range assessment.Answers {
				if ans.QuestionID == assessment.CurrentQuestionID {
					answeredCurrent = true
					break
				}
			}

			if answeredCurrent { 
				foundCurrentInList := false
				for i, qDef := range s.questions {
					if qDef.ID == assessment.CurrentQuestionID {
						foundCurrentInList = true
						if i+1 < len(s.questions) { 
							nextQ := s.questions[i+1]
							nextQuestionToShow = &nextQ
						} else { 
							assessment.Status = models.AssessmentStatusCompleted
							now := time.Now()
							assessment.CompletedAt = &now
							// CurrentQuestionID will be cleared before saving for completed assessment.
						}
						break
					}
				}
				if !foundCurrentInList { 
					log.Printf("CRITICAL: [AssessmentService] CurrentQuestionID %s (from assessment %d) not found in ordered question list, though it exists in questionsByID. This indicates a data inconsistency.", assessment.CurrentQuestionID, assessment.ID)
					return nil, assessment, fmt.Errorf("internal system error processing assessment %d", assessment.ID)
				}
			} else { 
				nextQuestionToShow = currentQFromDef
			}
		}
	}
	
	// Update assessment.CurrentQuestionID based on nextQuestionToShow or completion
	if assessment.Status == models.AssessmentStatusCompleted {
		assessment.CurrentQuestionID = "" // Clear for completed
	} else if nextQuestionToShow != nil {
		assessment.CurrentQuestionID = nextQuestionToShow.ID
	} // Else, if no next question and not completed (error case), CurrentQuestionID might remain as is or be cleared.

	updatedAssessment, errUpdate := s.repo.UpdateUserAssessment(assessment)
	if errUpdate != nil {
		errMsg := fmt.Sprintf("failed to update assessment state for assessmentID %d, userID %s (StartOrContinue)", assessment.ID, userID)
		log.Printf("ERROR: [AssessmentService] %s: %v", errMsg, errUpdate)
		return nextQuestionToShow, assessment, fmt.Errorf("%s: %w", errMsg, errUpdate)
	}

	if updatedAssessment.Status == models.AssessmentStatusCompleted || updatedAssessment.Status == models.AssessmentStatusCancelled {
		log.Printf("INFO: [AssessmentService] Assessment ID %d for userID '%s' has status '%s' after StartOrContinue logic. No next question.", updatedAssessment.ID, userID, updatedAssessment.Status)
		return nil, updatedAssessment, nil
	}
	
	if nextQuestionToShow == nil { 
		log.Printf("ERROR: [AssessmentService] Logic failed to determine next question for assessmentID %d (userID: %s, status: %s). This indicates an unexpected state.", updatedAssessment.ID, userID, updatedAssessment.Status)
		return nil, updatedAssessment, fmt.Errorf("internal error: unable to determine next assessment question for assessment %d", updatedAssessment.ID)
	}

	log.Printf("INFO: [AssessmentService] For userID '%s' (assessmentID %d), next question to show is ID '%s'.", userID, updatedAssessment.ID, nextQuestionToShow.ID)
	return nextQuestionToShow, updatedAssessment, nil
}

// SubmitAnswer processes a user's answer to a question and determines the next step.
func (s *assessmentService) SubmitAnswer(userID string, questionID string, answerValues []string) (*models.AssessmentQuestion, *models.UserAssessment, error) {
	log.Printf("INFO: [AssessmentService] UserID '%s' submitting answer for questionID '%s'.", userID, questionID)
	assessment, err := s.repo.GetUserAssessmentByUserID(userID, models.AssessmentStatusInProgress)
	if err != nil { 
		errMsg := fmt.Sprintf("failed to retrieve in-progress assessment for userID %s when submitting answer", userID)
		log.Printf("ERROR: [AssessmentService] %s: %v", errMsg, err)
		return nil, nil, fmt.Errorf("%s: %w", errMsg, err)
	}
	if assessment == nil {
		log.Printf("WARN: [AssessmentService] No in-progress assessment found for userID '%s' when submitting answer for questionID '%s'.", userID, questionID)
		return nil, nil, errors.New("you do not have an assessment in progress. Would you like to start one?")
	}

	if assessment.CurrentQuestionID == "" && questionID == s.questions[0].ID { 
		log.Printf("INFO: [AssessmentService] UserID '%s' submitting answer for first question '%s' with empty CurrentQuestionID. Setting CurrentQuestionID.", userID, questionID)
		assessment.CurrentQuestionID = questionID 
	} else if assessment.CurrentQuestionID != questionID {
		log.Printf("WARN: [AssessmentService] UserID '%s' attempted to answer question '%s', but current expected question is '%s' for assessmentID %d.", userID, questionID, assessment.CurrentQuestionID, assessment.ID)
		currentQFromDef, exists := s.questionsByID[assessment.CurrentQuestionID]
		if !exists || currentQFromDef == nil { 
			log.Printf("ERROR: [AssessmentService] Current expected question definition '%s' not found for assessmentID %d.", assessment.CurrentQuestionID, assessment.ID)
			return nil, assessment, fmt.Errorf("system error: current assessment question (ID: %s) not found. Please try starting the assessment again or contact support", assessment.CurrentQuestionID)
		}
		return currentQFromDef, assessment, fmt.Errorf("you seem to be answering a previous question. The current question is: '%s'", currentQFromDef.Text)
	}

	questionDef, exists := s.questionsByID[questionID]
	if !exists || questionDef == nil { 
		log.Printf("ERROR: [AssessmentService] Question definition for submitted questionID '%s' not found (assessmentID %d, userID %s).", questionID, assessment.ID, userID)
		return nil, assessment, fmt.Errorf("invalid question ID '%s' submitted", questionID)
	}

	if questionDef.IsRequired && (len(answerValues) == 0 || (len(answerValues) == 1 && strings.TrimSpace(answerValues[0]) == "")) {
		log.Printf("INFO: [AssessmentService] UserID '%s' did not provide an answer for required question '%s' (assessmentID %d).", userID, questionID, assessment.ID)
		return questionDef, assessment, errors.New("this question is required. Please provide an answer")
	}
	// TODO: Add more validation logic here (e.g., for single choice, ensure only one answer is provided; for multi-choice, ensure answers are from options).

	userVisibleMsg := processSpecialAnswers(assessment, questionDef, answerValues) 
	if userVisibleMsg != nil {
		if _, updateErr := s.repo.UpdateUserAssessment(assessment); updateErr != nil {
			log.Printf("ERROR: [AssessmentService] Failed to update assessment %d for user %s after special answer processing (e.g. cancellation): %v", assessment.ID, userID, updateErr)
		}
		return nil, assessment, userVisibleMsg 
	}

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

	var nextQuestionToShow *models.AssessmentQuestion
	currentIndexInList := -1
	for i, q := range s.questions {
		if q.ID == questionID {
			currentIndexInList = i
			break
		}
	}

	if currentIndexInList == -1 { 
		log.Printf("CRITICAL: QuestionID %s (assessmentID %d) not found in ordered question list after successful answer submission. Data inconsistency.", questionID, assessment.ID)
	    return nil, assessment, fmt.Errorf("internal system error processing question order for assessment %d", assessment.ID)
	}

	if currentIndexInList+1 < len(s.questions) { 
		nextQ := s.questions[currentIndexInList+1]
		nextQuestionToShow = &nextQ
		assessment.CurrentQuestionID = nextQ.ID
	} else { 
		assessment.Status = models.AssessmentStatusCompleted
		now := time.Now()
		assessment.CompletedAt = &now
		assessment.CurrentQuestionID = "" 
		log.Printf("INFO: [AssessmentService] AssessmentID %d for userID '%s' completed all questions.", assessment.ID, userID)
	}

	// Note: `processSpecialAnswers` might have already changed `assessment.Status`.
	// If status is now Cancelled, the next logic block for question progression might not be fully relevant,
	// but saving the answer is still important.
	updatedAssessment, errUpdate := s.repo.UpdateUserAssessment(assessment)
	if errUpdate != nil {
		errMsg := fmt.Sprintf("failed to update assessment %d for userID %s after submitting answer for questionID '%s'", assessment.ID, userID, questionID)
		log.Printf("ERROR: [AssessmentService] %s: %v", errMsg, errUpdate)
		return questionDef, assessment, fmt.Errorf("%s: %w", errMsg, errUpdate)
	}

	if updatedAssessment.Status == models.AssessmentStatusCompleted || updatedAssessment.Status == models.AssessmentStatusCancelled {
		log.Printf("INFO: [AssessmentService] Assessment %d for userID %s is now %s.", updatedAssessment.ID, userID, updatedAssessment.Status)
		return nil, updatedAssessment, nil 
	}

	// This state (nextQuestionToShow being nil when assessment is not completed/cancelled) should ideally not be reached.
	if nextQuestionToShow == nil { 
	    log.Printf("ERROR: [AssessmentService] Next question is unexpectedly nil for assessment %d (userID: %s, status: %s) after answering %s. This indicates a logic error.", updatedAssessment.ID, userID, updatedAssessment.Status, questionID)
	    return nil, updatedAssessment, fmt.Errorf("internal error: failed to determine next question for assessment %d", updatedAssessment.ID)
	}
	log.Printf("INFO: [AssessmentService] For userID '%s' (assessmentID %d), after answering '%s', next question to show is '%s'.", userID, updatedAssessment.ID, questionID, nextQuestionToShow.ID)
	return nextQuestionToShow, updatedAssessment, nil
}

// processSpecialAnswers handles answers to questions that might alter assessment flow (e.g., cancellation).
// It modifies the assessment status directly if needed and returns a user-facing error message if applicable.
func processSpecialAnswers(assessment *models.UserAssessment, question *models.AssessmentQuestion, answerValues []string) error {
	if question.ID == "q_welcome" && contains(answerValues, "No, next time") { // Matched English option
		assessment.Status = models.AssessmentStatusCancelled
		assessment.CurrentQuestionID = "" 
		log.Printf("INFO: [AssessmentService] UserID '%s' chose 'No, next time' for welcome question. AssessmentID %d cancelled.", assessment.UserID, assessment.ID)
		return errors.New("Okay, we respect your choice. You can restart the assessment anytime if you change your mind.") 
	}
	if question.ID == "q_privacy_consent" && contains(answerValues, "I do not agree") { // Matched English option
		assessment.Status = models.AssessmentStatusCancelled
		assessment.CurrentQuestionID = "" 
		log.Printf("INFO: [AssessmentService] UserID '%s' did not consent to privacy terms. AssessmentID %d cancelled.", assessment.UserID, assessment.ID)
		return errors.New("We take your privacy very seriously. If you do not agree to the processing of your information, we cannot proceed with personalized services. The assessment has been discontinued.") 
	}
	return nil
}

// GetAssessmentResult retrieves a completed assessment for a user.
func (s *assessmentService) GetAssessmentResult(userID string) (*models.UserAssessment, error) {
	if userID == "" {
		log.Println("WARN: [AssessmentService] GetAssessmentResult called with empty userID.")
		return nil, errors.New("userID cannot be empty")
	}
	log.Printf("INFO: [AssessmentService] Attempting to get completed assessment result for userID: %s", userID)
	
	assessment, err := s.repo.GetUserAssessmentByUserID(userID, models.AssessmentStatusCompleted)
	if err != nil {
		errMsg := fmt.Sprintf("failed to get completed assessment for userID %s from repository", userID)
		log.Printf("ERROR: [AssessmentService] %s: %v", errMsg, err)
		return nil, fmt.Errorf("%s: %w", errMsg, err)
	}
	if assessment == nil { 
		log.Printf("INFO: [AssessmentService] No completed assessment found for userID '%s'.", userID)
		return nil, nil 
	}
	log.Printf("INFO: [AssessmentService] Successfully retrieved completed assessment ID %d for userID '%s'.", assessment.ID, userID)
	return assessment, nil
}

// contains is a helper function to check if a string slice contains a specific item.
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}