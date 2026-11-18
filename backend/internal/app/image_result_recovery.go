package app

import (
	"luckytv/backend/internal/model"
)

func (s *Service) recoverAcceptedImage(task model.Task) (*model.Task, bool, error) {
	return s.mediaAttempts().RecoverAcceptedImage(task)
}
