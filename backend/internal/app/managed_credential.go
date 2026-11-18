package app

import (
	"luckytv/backend/internal/luckytv"
	"luckytv/backend/internal/generation"
)

const managedLuckyTVRef = luckytv.CredentialRef

func (s *Service) SetLuckyTV(connection *luckytv.Service) {
	if s != nil {
		s.luckyTV = connection
	}
}

func (s *Service) LuckyTV() *luckytv.Service {
	if s == nil {
		return nil
	}
	return s.luckyTV
}

func (s *Service) resolveManagedLuckyTVSecrets(input map[string]any) (map[string]any, error) {
	return generation.ResolveManagedLuckyTVSecrets(input, s.luckyTV, s.ReadLocalModelConfig)
}

func (s *Service) lookupLuckyTVCredential() (apiKey, baseURL, accountID, tokenID string, err error) {
	return generation.LookupLuckyTVCredential(s.luckyTV, s.ReadLocalModelConfig)
}

func (s *Service) resolveChannelModelsRequest(input *ChannelModelsRequest) error {
	return generation.ResolveChannelModelsRequest(input, s.luckyTV, s.ReadLocalModelConfig)
}

func (s *Service) ResolveCustomRelayAPIKey(targetURL, incoming string) (string, error) {
	return generation.ResolveCustomRelayKey(targetURL, incoming, s.luckyTV, s.ReadLocalModelConfig)
}

func (s *Service) resolveCustomRelayKey(targetURL, incoming string) (string, error) {
	return generation.ResolveCustomRelayKey(targetURL, incoming, s.luckyTV, s.ReadLocalModelConfig)
}

func (s *Service) noteLuckyTVProviderError(message string) {
	if s != nil {
		generation.NoteLuckyTVProviderError(s.luckyTV, message)
	}
}
