package master

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveMasterDir(t *testing.T) {
	tmpDir := t.TempDir()
	masterDir := filepath.Join(tmpDir, "master")
	os.Mkdir(masterDir, 0755)

	t.Run("explicit_path", func(t *testing.T) {
		got, err := ResolveMasterDir(masterDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != masterDir {
			t.Errorf("got %q, want %q", got, masterDir)
		}
	})

	t.Run("env_var", func(t *testing.T) {
		os.Setenv("PHOTO_AI_MASTER_DIR", masterDir)
		defer os.Unsetenv("PHOTO_AI_MASTER_DIR")

		got, err := ResolveMasterDir("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != masterDir {
			t.Errorf("got %q, want %q", got, masterDir)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		// Clear env
		os.Unsetenv("PHOTO_AI_MASTER_DIR")
		
		// In test environment, executable adjacent master/ might not exist.
		// We just verify it returns an error if nothing is found.
		_, err := ResolveMasterDir("")
		if err == nil {
			t.Error("expected error when no master dir exists, got nil")
		}
	})
}
