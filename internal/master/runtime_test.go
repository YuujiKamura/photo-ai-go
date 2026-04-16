package master

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveMasterSource(t *testing.T) {
	tmpDir := t.TempDir()
	userMasterDir := filepath.Join(tmpDir, "user-master")
	os.MkdirAll(userMasterDir, 0755)

	// Create a valid manifest and dummy CSV
	manifest := Manifest{
		SchemaVersion: 1,
		MasterVersion: "1.0",
		Files:         []string{"test.csv"},
	}
	WriteManifestToRoot(userMasterDir, manifest)
	os.WriteFile(filepath.Join(userMasterDir, "test.csv"), []byte("header\ndata"), 0644)

	t.Run("env_var", func(t *testing.T) {
		t.Setenv("PHOTO_AI_MASTER_DIR", userMasterDir)
		src, err := ResolveMasterSource(t.TempDir())
		if err != nil {
			t.Fatalf("ResolveMasterSource: %v", err)
		}
		if src.Source != SourceUser {
			t.Errorf("expected user source, got %s", src.Source)
		}
		if src.RootDir != userMasterDir {
			t.Errorf("got %q, want %q", src.RootDir, userMasterDir)
		}
	})

	t.Run("workspace", func(t *testing.T) {
		t.Setenv("PHOTO_AI_MASTER_DIR", "")
		t.Setenv("LOCALAPPDATA", filepath.Join(t.TempDir(), "nonexistent"))
		t.Setenv("APPDATA", filepath.Join(t.TempDir(), "nonexistent"))
		workspaceDir := t.TempDir()
		masterDir := filepath.Join(workspaceDir, "master")
		os.MkdirAll(masterDir, 0755)
		WriteManifestToRoot(masterDir, manifest)
		os.WriteFile(filepath.Join(masterDir, "test.csv"), []byte("header\ndata"), 0644)

		src, err := ResolveMasterSource(workspaceDir)
		if err != nil {
			t.Fatalf("ResolveMasterSource: %v", err)
		}
		if src.Source != SourceWorkspace {
			t.Errorf("expected workspace source, got %s", src.Source)
		}
	})
}
