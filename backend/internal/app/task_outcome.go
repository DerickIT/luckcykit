package app

import (
	"luckytv/backend/internal/model"
	localtask "luckytv/backend/internal/task"
)

func (s *Service) taskOutcomeFacts(tasks []model.Task) map[string]localtask.OutcomeFacts {
	return localtask.OutcomeFactsForTasks(s.repo, tasks, taskStageRequestNotSent)
}
