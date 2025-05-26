package utils

import (
	"crypto/md5"
	"encoding/hex"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// SendJSONError sends a standardized JSON error response and logs the internal error.
// For 5xx errors, it sends a generic public message while logging the actual internalError.
// For 4xx errors, the publicMsg is shown to the client, and internalError (if provided) is logged.
func SendJSONError(c *gin.Context, statusCode int, publicMsg string, internalError error, details ...string) {
	errorDetails := ""
	if len(details) > 0 {
		errorDetails = details[0] // Taking the first detail if multiple are provided for simplicity
	}

	response := gin.H{"error": publicMsg}
	if errorDetails != "" {
		response["details"] = errorDetails
	}

	// Log the error with more details internally
	if internalError != nil {
		log.Printf("ERROR: Handler error: status_code=%d, public_message='%s', internal_error='%v', details='%s', path='%s'",
			statusCode, publicMsg, internalError, errorDetails, c.Request.URL.Path)
	} else {
		// Log even if internalError is nil, for client errors that might be of interest
		log.Printf("INFO: Handler response: status_code=%d, public_message='%s', details='%s', path='%s'",
			statusCode, publicMsg, errorDetails, c.Request.URL.Path)
	}

	// For 5xx errors, ensure the public message is generic if not already.
	// The actual sensitive error is logged above and not sent to client.
	if statusCode >= http.StatusInternalServerError && publicMsg == "" {
		response["error"] = "An unexpected error occurred. Please try again later."
	} else if statusCode >= http.StatusInternalServerError && internalError != nil && publicMsg == internalError.Error() {
		// If publicMsg is the same as internalError for a 5xx, replace publicMsg with generic one.
		response["error"] = "An unexpected error occurred. Please try again later."
		log.Printf("WARN: For 5xx error, public message was same as internal error. Replaced with generic message for client. Original internal error: %v", internalError)
	}


	c.AbortWithStatusJSON(statusCode, response)
}

// GenerateID 生成唯一ID
func GenerateID() string {
	timestamp := time.Now().UnixNano()
	hash := md5.Sum([]byte(time.Now().String()))
	return hex.EncodeToString(hash[:]) + string(timestamp)
}

// FormatTime 格式化时间
func FormatTime(t time.Time) string {
	return t.Format("2006-01-02 15:04:05")
}

// ParseCronExpression 解析Cron表达式
func ParseCronExpression(expr string) (bool, error) {
	// 这里可以添加Cron表达式解析逻辑
	// 简单实现，实际应使用cron库
	return true, nil
}
