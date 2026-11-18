package app

import (
	"luckytv/backend/internal/generation"
	"luckytv/backend/internal/model"
	"luckytv/backend/internal/modelcatalog"
)

func (s *Service) ValidateTaskCapability(input map[string]any) error {
	return generation.ValidateTaskCapability(input, s.resolveVideoCapability, s.channelModelLookup())
}

func (s *Service) channelModelLookup() modelcatalog.ChannelModelLookup {
	return func(channelID, modelKey string) (*model.ChannelModel, error) {
		return s.repo.ChannelModelByKey(channelID, modelKey)
	}
}
