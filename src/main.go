package main

import (
	"log"

	"github.com/gin-gonic/gin"

	"project/api"
	"project/config"
	"project/database" // Added database import
	"project/middleware"
	"project/models"     // Added models import
	"project/repository" // Added repository import
	"project/services"   // Added services import

	"gorm.io/gorm" // Added gorm import
)

func main() {
	// Load application configuration
	config.LoadConfig() // Log prefixes are handled within LoadConfig

	// Initialize database connection
	db, err := database.Init() // Log prefixes are handled within Init
	if err != nil {
		log.Fatalf("FATAL: [Main] Failed to initialize database: %v", err)
	}

	// Auto-migrate database schema
	runMigrations(db) // Log prefixes are handled within runMigrations

	// Initialize Repositories
	// Assuming ChatRepository and AssessmentRepository are in-memory or do not require DB for now.
	// This might need adjustment if they are converted to use GORM.
	chatRepo := repository.NewChatRepository()       
	assessmentRepo := repository.NewAssessmentRepository() 
	quotaRepo := repository.NewQuotaRepository(db)     
	planRepo := repository.NewPlanRepository(db)       
	log.Println("INFO: [Main] Repositories initialized.")

	// Initialize Services
	assessmentService := services.NewAssessmentService(assessmentRepo)
	schedulerService := services.NewSchedulerService(assessmentRepo) 
	chatService := services.NewChatService(chatRepo)                 
	planService := services.NewPlanService(planRepo, assessmentRepo) 
	log.Println("INFO: [Main] Services initialized.")

	// Initialize API Handler with all dependencies
	apiHandler := api.NewAPIHandler(
		chatRepo,
		assessmentRepo,
		quotaRepo,
		planRepo, 
		assessmentService,
		schedulerService,
		chatService,
		planService, 
		db,          
	)
	log.Println("INFO: [Main] API Handler initialized.")

	// Create Gin engine
	r := gin.Default()

	// Set trusted proxies if running behind one (e.g., Nginx, Load Balancer)
	// r.SetTrustedProxies([]string{"127.0.0.1"}) // Example, adjust as needed for deployment
	r.SetTrustedProxies(nil) // Set to nil if not using trusted proxies or for default behavior

	// Register middlewares
	r.Use(middleware.Logger()) // Custom logger middleware
	r.Use(middleware.Cors())   // CORS middleware
	log.Println("INFO: [Main] Middlewares registered.")

	// Register routes
	registerRoutes(r, apiHandler) 
	log.Println("INFO: [Main] Routes registered.")

	// Start the server
	serverPort := ":" + config.AppConfig.Server.Port
	if config.AppConfig.Server.Port == "" { // Check if port in config is empty
		log.Println("WARN: [Main] Server port not configured, using default :8080.")
		serverPort = ":8080" 
	}
	log.Printf("INFO: [Main] Starting server on port %s", serverPort)
	if err := r.Run(serverPort); err != nil {
		log.Fatalf("FATAL: [Main] Server failed to start: %v", err)
	}
}

func runMigrations(db *gorm.DB) {
	log.Println("INFO: [Main] Running database migrations...")
	err := db.AutoMigrate(
		&models.ChatMessage{},       
		&models.UserAssessment{},    
		&models.AssessmentQuestion{}, 
		&models.GuestQuota{},         
		&models.Plan{},               
		&models.PlanTask{},           
		// Add other models here as needed
	)
	if err != nil {
		log.Fatalf("FATAL: [Main] Failed to auto-migrate database: %v", err)
	}
	log.Println("INFO: [Main] Database migration completed.")
}

func registerRoutes(r *gin.Engine, handler *api.APIHandler) { 
	// API route group
	apiGroup := r.Group("/api")
	{
		// Initialization endpoint
		apiGroup.GET("/init", handler.InitHandler) 
		// Chat related endpoint
		apiGroup.POST("/chat", handler.ChatHandler) 
		
		// Plan related endpoints
		planGroup := apiGroup.Group("/plan")
		{
			planGroup.POST("/generate", handler.GeneratePlanHandler)             
			planGroup.GET("/user/:userID", handler.GetPlansForUserHandler)       
			planGroup.GET("/:planID", handler.GetPlanDetailsHandler)             
			planGroup.POST("/task/:taskID/complete", handler.CompleteTaskHandler) 
			planGroup.POST("/task/:taskID/skip", handler.SkipTaskHandler)         
		}
		// Example for a scheduler-specific endpoint if needed in future
		// schedulerGroup := apiGroup.Group("/scheduler")
		// {
		// 	// Define scheduler routes here
		// }
	}
}
