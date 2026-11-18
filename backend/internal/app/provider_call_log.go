package app

import (
	"luckytv/backend/internal/generation"
	"luckytv/backend/internal/model"
)

func (s *Service) EnrichAPICallLog(log *model.ApiCallLog, responseBody []byte) {
	generation.EnrichAPICallLog(log, responseBody)
}
