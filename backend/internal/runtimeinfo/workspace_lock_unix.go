//go:build !windows

package runtimeinfo

import (
	"luckytv/backend/internal/desktopstorage"
	"os"
)

func lockWorkspaceFile(dataDir string) (*os.File, error) { return desktopstorage.Lock(dataDir) }
