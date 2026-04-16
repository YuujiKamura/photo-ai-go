package master

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	SourceUser      = "user"
	SourceWorkspace = "workspace"
)

type Source struct {
	RootDir       string
	Source        string
	Writable      bool
	Manifest      Manifest
	SchemaVersion int
	MasterVersion string
}

// ResolveMasterSource attempts to find a valid master directory.
func ResolveMasterSource(repoDir string) (Source, error) {
	// 1. Try environment path if explicitly set
	if env := os.Getenv("PHOTO_AI_MASTER_DIR"); env != "" {
		if src, ok := loadSourceFromRoot(env, SourceUser, true); ok {
			return src, nil
		}
	}

	// 2. Try default user root
	if userRoot, err := userMasterRoot(); err == nil {
		if src, ok := loadSourceFromRoot(userRoot, SourceUser, true); ok {
			return src, nil
		}
	}

	// 3. Try relative to repository/executable
	if src, ok := loadSourceFromRoot(filepath.Join(repoDir, "master"), SourceWorkspace, true); ok {
		return src, nil
	}

	return Source{}, fmt.Errorf("no usable master source found")
}

func userMasterRoot() (string, error) {
	base := os.Getenv("PHOTO_AI_MASTER_DIR")
	if base == "" {
		base = os.Getenv("LOCALAPPDATA")
		if base == "" {
			var err error
			base, err = os.UserConfigDir()
			if err != nil {
				return "", fmt.Errorf("resolve user config dir: %w", err)
			}
		}
		base = filepath.Join(base, "photo-ai", "master")
	}
	return base, nil
}

func loadSourceFromRoot(root string, sourceName string, writable bool) (Source, bool) {
	manifest, err := ReadManifestFromRoot(root)
	if err != nil {
		return Source{}, false
	}
	for _, name := range manifest.Files {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(name))); err != nil {
			return Source{}, false
		}
	}
	return Source{
		RootDir:       root,
		Source:        sourceName,
		Writable:      writable,
		Manifest:      manifest,
		SchemaVersion: manifest.SchemaVersion,
		MasterVersion: manifest.MasterVersion,
	}, true
}
