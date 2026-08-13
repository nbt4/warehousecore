package handlers

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveLegacyPNGLabelCache(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	nested := filepath.Join(root, "product")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	pngPath := filepath.Join(nested, "old-label.PNG")
	pdfPath := filepath.Join(nested, "current-label.pdf")
	for path, content := range map[string]string{pngPath: "png", pdfPath: "%PDF-1.7"} {
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	if err := removeLegacyPNGLabelCache(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(pngPath); !os.IsNotExist(err) {
		t.Fatalf("legacy PNG still exists: %v", err)
	}
	if _, err := os.Stat(pdfPath); err != nil {
		t.Fatalf("PDF cache was removed: %v", err)
	}
}
