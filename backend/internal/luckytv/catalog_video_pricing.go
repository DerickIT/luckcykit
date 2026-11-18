package luckytv

import (
	"encoding/json"

	"luckytv/backend/internal/modelcatalog"
)

func NormalizeCatalogVideoPricing(raw json.RawMessage) (map[string]any, bool) {
	return modelcatalog.NormalizeCatalogVideoPricing(raw)
}
