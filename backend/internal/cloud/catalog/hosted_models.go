package catalog

import (
	"sort"
	"strings"

	"luckytv/backend/internal/cloud/store"
)

// HostedModelInfo is the user-facing projection of a hosted model. It contains
// only fields needed to choose and price a model; upstream routing data remains
// inside the cloud task service.
type HostedModelInfo struct {
	ID                     string   `json:"id"`
	DisplayName            string   `json:"displayName"`
	Capabilities           []string `json:"capabilities"`
	Enabled                bool     `json:"enabled"`
	PriceVersion           string   `json:"priceVersion"`
	MaxInputTokens         int64    `json:"maxInputTokens"`
	MaxOutputTokens        int64    `json:"maxOutputTokens"`
	InputPointsPerMillion  int64    `json:"inputPointsPerMillion"`
	OutputPointsPerMillion int64    `json:"outputPointsPerMillion"`
}

type HostedModelService struct {
	models []HostedModelInfo
}

func NewHostedModelService(models []store.HostedModel) *HostedModelService {
	visible := make([]HostedModelInfo, 0, len(models))
	for _, model := range models {
		if !model.Enabled || strings.TrimSpace(model.ID) == "" {
			continue
		}
		visible = append(visible, HostedModelInfo{
			ID: model.ID, DisplayName: model.ID, Capabilities: []string{"text"}, Enabled: true,
			PriceVersion: model.PriceVersion, MaxInputTokens: model.MaxInputTokens,
			MaxOutputTokens: model.MaxOutputTokens, InputPointsPerMillion: model.InputPointsPerMillion,
			OutputPointsPerMillion: model.OutputPointsPerMillion,
		})
	}
	sort.Slice(visible, func(i, j int) bool { return visible[i].ID < visible[j].ID })
	return &HostedModelService{models: visible}
}

func (s *HostedModelService) List() []HostedModelInfo {
	if s == nil {
		return []HostedModelInfo{}
	}
	result := make([]HostedModelInfo, len(s.models))
	copy(result, s.models)
	return result
}
