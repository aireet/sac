package response

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Error sends a JSON error response. If err is provided, its message is appended.
func Error(c *gin.Context, status int, msg string, errs ...error) {
	if len(errs) > 0 && errs[0] != nil {
		msg = fmt.Sprintf("%s: %v", msg, errs[0])
	}
	c.JSON(status, gin.H{"error": msg})
}

// BadRequest sends a 400 error response.
func BadRequest(c *gin.Context, msg string, errs ...error) {
	Error(c, http.StatusBadRequest, msg, errs...)
}

// Unauthorized sends a 401 error response.
func Unauthorized(c *gin.Context, msg string) {
	Error(c, http.StatusUnauthorized, msg)
}

// Forbidden sends a 403 error response.
func Forbidden(c *gin.Context, msg string) {
	Error(c, http.StatusForbidden, msg)
}

// NotFound sends a 404 error response.
func NotFound(c *gin.Context, msg string, errs ...error) {
	Error(c, http.StatusNotFound, msg, errs...)
}

// Conflict sends a 409 error response.
func Conflict(c *gin.Context, msg string) {
	Error(c, http.StatusConflict, msg)
}

// ServiceUnavailable sends a 503 error response.
func ServiceUnavailable(c *gin.Context, msg string) {
	Error(c, http.StatusServiceUnavailable, msg)
}

// InternalError sends a 500 error response with the underlying error detail.
func InternalError(c *gin.Context, msg string, err error) {
	Error(c, http.StatusInternalServerError, msg, err)
}

// OK sends a 200 JSON response with data.
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, data)
}

// Created sends a 201 JSON response with data.
func Created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, data)
}

// Success sends a 200 JSON response with a message.
func Success(c *gin.Context, msg string) {
	c.JSON(http.StatusOK, gin.H{"message": msg})
}
