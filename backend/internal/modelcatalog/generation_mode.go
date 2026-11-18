package modelcatalog

import "luckytv/backend/internal/canvas/capability"

func GenerationModeSupported(mode string) bool {
	return capability.BuiltinRegistry().SupportsGenerationMode(normalizeCapability(mode))
}
