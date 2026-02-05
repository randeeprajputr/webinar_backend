package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Body is the standard API response envelope.
type Body struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string     `json:"error,omitempty"`
}

// OK sends a 200 JSON response with data.
func OK(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Body{Success: true, Data: data})
}

// Created sends a 201 JSON response with data.
func Created(c *gin.Context, data interface{}) {
	c.JSON(http.StatusCreated, Body{Success: true, Data: data})
}

// NoContent sends 204.
func NoContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

// BadRequest sends 400 with error message.
func BadRequest(c *gin.Context, err string) {
	c.JSON(http.StatusBadRequest, Body{Success: false, Error: err})
}

// Unauthorized sends 401.
func Unauthorized(c *gin.Context, err string) {
	c.JSON(http.StatusUnauthorized, Body{Success: false, Error: err})
}

// Forbidden sends 403.
func Forbidden(c *gin.Context, err string) {
	c.JSON(http.StatusForbidden, Body{Success: false, Error: err})
}

// NotFound sends 404.
func NotFound(c *gin.Context, err string) {
	c.JSON(http.StatusNotFound, Body{Success: false, Error: err})
}

// Conflict sends 409.
func Conflict(c *gin.Context, err string) {
	c.JSON(http.StatusConflict, Body{Success: false, Error: err})
}

// ServiceUnavailable sends 503.
func ServiceUnavailable(c *gin.Context, err string) {
	c.JSON(http.StatusServiceUnavailable, Body{Success: false, Error: err})
}

// Internal sends 500.
func Internal(c *gin.Context, err string) {
	c.JSON(http.StatusInternalServerError, Body{Success: false, Error: err})
}
