package app

import "luckytv/backend/internal/assistantturns"

func (s *Service) ClaimAssistantContinuation(userID, id string) (*assistantturns.ContinuationView, error) {
	return s.assistantTurnsOrInit().ClaimContinuation(userID, id, s.prepareAssistantProduction)
}
