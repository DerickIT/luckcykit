package app

import "luckytv/backend/internal/generation"

func EnsureChatCompletionStreamUsageRequest(data []byte) ([]byte, error) {
	return generation.EnsureChatCompletionStreamUsageRequest(data)
}
