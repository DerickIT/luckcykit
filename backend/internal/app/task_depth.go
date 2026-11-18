package app

import (
	"luckytv/backend/internal/model"
	localtask "luckytv/backend/internal/task"
)

type DepthCaptureCreateRequest = localtask.DepthCaptureCreateRequest

func (s *Service) CreateDepthCaptureTask(userID string, req DepthCaptureCreateRequest) (*model.Task, error) {
	return s.taskDomain().CreateDepthCaptureTask(userID, req)
}
