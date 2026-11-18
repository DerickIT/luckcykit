package app

import (
	"luckytv/backend/internal/mediaattempt"
)

func definiteImageInputRejection(err error) bool {
	return mediaattempt.DefiniteImageInputRejection(err)
}
