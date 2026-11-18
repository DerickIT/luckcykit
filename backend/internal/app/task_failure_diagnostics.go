package app

import (
	"luckytv/backend/internal/generation"
	"luckytv/backend/internal/model"
)

func captureTaskFailureDiagnostics(task *model.Task, err error, source string) {
	generation.CaptureTaskFailureDiagnostics(task, err, source)
}
