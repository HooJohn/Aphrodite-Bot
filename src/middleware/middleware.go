package middleware

import (
	"fmt"
	"log"
	"time"

	"github.com/gin-gonic/gin"
)

// Logger is a Gin middleware for logging HTTP requests and responses.
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Start timer
		startTime := time.Now()

		// Process request
		c.Next()

		// End timer
		latency := time.Since(startTime)

		// Request details
		method := c.Request.Method
		uri := c.Request.RequestURI
		statusCode := c.Writer.Status()
		clientIP := c.ClientIP()
		// Get errors written by subsequent handlers
		errorsStr := c.Errors.ByType(gin.ErrorTypePrivate).String()
		if errorsStr == "" {
			errorsStr = "None"
		}
		
		// Set header for response time
		c.Writer.Header().Set("X-Response-Time", latency.String())

		// Log format (using standard log package for consistency with other app logs)
		// Using [GIN] prefix similar to Gin's default logger, but with more structure.
		log.Printf("[GIN] %s | %3d | %13v | %15s | %-7s %s\n      Errors: %s",
			startTime.Format("2006/01/02 - 15:04:05"),
			statusCode,
			latency,
			clientIP,
			method,
			uri,
			errorsStr,
		)
	}
}

// Cors is a Gin middleware for enabling Cross-Origin Resource Sharing (CORS).
// It allows requests from any origin.
func Cors() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*") // Allow all origins
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With, User-Agent") // Added User-Agent
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE, PATCH") // Added PATCH

		// Handle preflight requests (OPTIONS)
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204) // No Content
			return
		}

		c.Next()
	}
}
