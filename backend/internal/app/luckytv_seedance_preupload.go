package app

import (
	"context"

	"luckytv/backend/internal/generation"
)

func (s *Service) prepareLuckyTVSeedanceReferences(ctx context.Context, userID string, input *canvasGenerationInput) error {
	return generation.PrepareLuckyTVSeedanceReferencesForUser(ctx, userID, input, s.OpenResource)
}
