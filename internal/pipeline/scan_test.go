package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestScanImages(t *testing.T) {
	dir := t.TempDir()
	
	// Create some dummy files
	files := []string{"a.jpg", "b.png", "c.txt", "sub/d.jpeg"}
	for _, f := range files {
		p := filepath.Join(dir, f)
		os.MkdirAll(filepath.Dir(p), 0755)
		os.WriteFile(p, []byte{}, 0644)
	}

	t.Run("non-recursive", func(t *testing.T) {
		imgs, err := ScanImages(context.Background(), dir, false, false)
		if err != nil {
			t.Fatalf("ScanImages: %v", err)
		}
		if len(imgs) != 2 { // a.jpg, b.png
			t.Errorf("expected 2 images, got %d", len(imgs))
		}
	})

	t.Run("recursive", func(t *testing.T) {
		imgs, err := ScanImages(context.Background(), dir, true, false)
		if err != nil {
			t.Fatalf("ScanImages: %v", err)
		}
		if len(imgs) != 3 { // a.jpg, b.png, d.jpeg
			t.Errorf("expected 3 images, got %d", len(imgs))
		}
	})

	t.Run("include-all", func(t *testing.T) {
		imgs, err := ScanImages(context.Background(), dir, false, true)
		if err != nil {
			t.Fatalf("ScanImages: %v", err)
		}
		if len(imgs) != 3 { // a.jpg, b.png, c.txt
			t.Errorf("expected 3 files, got %d", len(imgs))
		}
	})
}

func TestIsExcludedFolder(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"C:/Users/test/非使用/photo.jpg", true},
		{"C:/Users/test/hisiyou/photo.jpg", true},
		{"C:/Users/test/work/photo.jpg", false},
	}

	for _, tt := range tests {
		if got := isExcludedFolder(tt.path); got != tt.want {
			t.Errorf("isExcludedFolder(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}
