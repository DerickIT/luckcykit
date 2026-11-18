package app

import "luckytv/backend/internal/generation"

func (s *Service) validatePortraitTaskQuote(input map[string]any) error {
	return generation.ValidatePortraitTaskQuote(s.dataDir, input, s.lookupLuckyTVCredential)
}
