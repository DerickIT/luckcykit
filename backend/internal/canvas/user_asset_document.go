package canvas

import (
	"encoding/json"

	"luckytv/backend/internal/asset"
)

func validateUserAssetDocument(raw json.RawMessage) error {
	return asset.ValidateDocument(raw)
}
