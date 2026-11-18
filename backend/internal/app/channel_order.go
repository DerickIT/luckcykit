package app

import (
	"luckytv/backend/internal/model"
	"luckytv/backend/internal/modelcatalog"
)

type ChannelOrderItem = modelcatalog.ChannelOrderItem
type ChannelOrderRequest = modelcatalog.ChannelOrderRequest

func (s *Service) AdminChannelOrder(actor *model.User, channelID string) ([]ChannelOrderItem, error) {
	return modelcatalog.AdminChannelOrder(s.repo, s.RequireAdmin, actor, channelID)
}

func (s *Service) SaveAdminChannelOrder(actor *model.User, channelID string, req ChannelOrderRequest) error {
	return modelcatalog.SaveAdminChannelOrder(s.repo, s.RequireAdmin, actor, channelID, req, s.invalidateRouteCatalog)
}
