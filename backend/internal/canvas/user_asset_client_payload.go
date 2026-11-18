package canvas

import (
	"encoding/json"

	"luckytv/backend/internal/asset"
	"luckytv/backend/internal/model"
)

func ClientAssetPayload(item model.Asset) json.RawMessage {
	return asset.ClientAssetPayload(item)
}
