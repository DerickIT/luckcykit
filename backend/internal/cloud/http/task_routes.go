package httpapi

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"

	"luckytv/backend/internal/cloud/ledger"
	"luckytv/backend/internal/cloud/modelgateway"
	"luckytv/backend/internal/cloud/task"
)

type taskRoutes struct {
	tasks *task.Service
}

func (a *taskRoutes) submit(c *gin.Context) {
	var body struct {
		ModelID         string                 `json:"modelId"`
		Messages        []modelgateway.Message `json:"messages"`
		MaxOutputTokens int64                  `json:"maxOutputTokens"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.ModelID) == "" || len(body.Messages) == 0 || body.MaxOutputTokens < 0 {
		c.JSON(400, gin.H{"error": "invalid_request"})
		return
	}
	result, err := a.tasks.Submit(c.Request.Context(), task.SubmitRequest{
		AccountID: c.GetString(accountContextKey), IdempotencyKey: c.GetHeader("Idempotency-Key"), ModelID: body.ModelID,
		Request: modelgateway.Request{Messages: body.Messages, MaxOutputTokens: body.MaxOutputTokens},
	})
	switch {
	case errors.Is(err, task.ErrTaskIdempotency):
		c.JSON(409, gin.H{"error": "idempotency_key_conflict"})
	case errors.Is(err, task.ErrModelUnavailable):
		c.JSON(422, gin.H{"error": "model_unavailable"})
	case errors.Is(err, ledger.ErrInsufficientPoints):
		c.JSON(402, gin.H{"error": "insufficient_points"})
	case err != nil:
		c.JSON(500, gin.H{"error": "task_submit_failed"})
	default:
		c.JSON(202, gin.H{"task": result.Task, "replayed": result.Replayed})
	}
}

func (a *taskRoutes) get(c *gin.Context) {
	result, err := a.tasks.Get(c.GetString(accountContextKey), c.Param("taskID"))
	if errors.Is(err, task.ErrTaskNotFound) {
		c.JSON(404, gin.H{"error": "task_not_found"})
		return
	}
	if err != nil {
		c.JSON(500, gin.H{"error": "task_unavailable"})
		return
	}
	c.JSON(200, gin.H{"task": result})
}
