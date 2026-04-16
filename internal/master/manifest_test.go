package master

import (
	"testing"
)

func TestReadManifestFromRoot(t *testing.T) {
	tmpDir := t.TempDir()
	manifest := Manifest{
		SchemaVersion: 1,
		MasterVersion: "1.0",
		Files:         []string{"test.csv"},
	}
	if err := WriteManifestToRoot(tmpDir, manifest); err != nil {
		t.Fatalf("WriteManifestToRoot: %v", err)
	}

	got, err := ReadManifestFromRoot(tmpDir)
	if err != nil {
		t.Fatalf("ReadManifestFromRoot: %v", err)
	}
	if got.MasterVersion != "1.0" {
		t.Errorf("got version %q, want 1.0", got.MasterVersion)
	}
}
