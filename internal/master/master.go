package master

import (
	"fmt"
	"os"
	"path/filepath"
)

// ResolveMasterDir finds the master data directory on the filesystem.
// Priority:
// 1. Explicit path (if provided)
// 2. Environment variable PHOTO_AI_MASTER_DIR
// 3. "master" directory next to the executable
// 4. "master" directory in current working directory
func ResolveMasterDir(explicitPath string) (string, error) {
	if explicitPath != "" {
		if info, err := os.Stat(explicitPath); err == nil && info.IsDir() {
			return explicitPath, nil
		}
		// If explicit path is a file, return as is (matching package handles single CSV)
		return explicitPath, nil
	}

	if env := os.Getenv("PHOTO_AI_MASTER_DIR"); env != "" {
		if info, err := os.Stat(env); err == nil && info.IsDir() {
			return env, nil
		}
	}

	exe, err := os.Executable()
	exeDir := ""
	if err == nil {
		exeDir = filepath.Dir(exe)
	}

	cwd, _ := os.Getwd()
	paths := []string{
		filepath.Join(exeDir, "master"),
		filepath.Join(cwd, "master"),
	}

	for _, p := range paths {
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			return p, nil
		}
	}

	return "", fmt.Errorf("master directory not found")
}
