package app

import (
	cw "luckytv/backend/internal/canvasworkflow"
	"luckytv/backend/internal/model"
)

func workflowBad(message string) error { return cw.BadRequest(message) }

func (s *Service) workflowResource(user, id, kind string) (*model.Resource, error) {
	return cw.ReadyResource(s.repo, user, id, kind)
}

func workflowReferenceID(v any) string { return cw.ReferenceID(v) }

func workflowResourceIDs(value any) map[string]bool { return cw.ReferenceIDs(value) }
