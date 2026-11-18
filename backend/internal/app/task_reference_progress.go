package app

import (
	"luckytv/backend/internal/generation"
	localtask "luckytv/backend/internal/task"
)

type appReferenceProgressPort = localtask.ReferenceProgressPort

func (s *Service) newReferenceProgressPort(taskID string) *appReferenceProgressPort {
	return localtask.NewReferenceProgressPort(&s.referenceProgress, taskID)
}

func (s *Service) referenceProgressForTask(taskID string) *generation.ReferenceProgress {
	return localtask.ReferenceProgressForTask(&s.referenceProgress, taskID)
}
