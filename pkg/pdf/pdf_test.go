package pdf

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPageCount(t *testing.T) {
	pdfPath := filepath.Join("..", "..", "examples", "2603.29199v1.pdf")
	if _, err := os.Stat(pdfPath); err != nil {
		t.Skip("test PDF not available")
	}

	count, err := PageCount(pdfPath)
	if err != nil {
		t.Fatalf("PageCount failed: %v", err)
	}
	if count <= 0 {
		t.Errorf("expected positive page count, got %d", count)
	}
	t.Logf("PDF has %d pages", count)
}

func TestPageCount_NonExistent(t *testing.T) {
	_, err := PageCount("/nonexistent/file.pdf")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestExtractPages(t *testing.T) {
	pdfPath := filepath.Join("..", "..", "examples", "2603.29199v1.pdf")
	if _, err := os.Stat(pdfPath); err != nil {
		t.Skip("test PDF not available")
	}

	tmpDir := t.TempDir()
	pages, err := ExtractPages(pdfPath, 150, tmpDir)
	if err != nil {
		t.Fatalf("ExtractPages failed: %v", err)
	}

	if len(pages) == 0 {
		t.Fatal("expected at least one page")
	}

	for i, page := range pages {
		if _, err := os.Stat(page); err != nil {
			t.Errorf("page %d file not found: %s", i+1, page)
		}
	}
	t.Logf("Extracted %d pages", len(pages))
}

func TestExtractPages_DefaultDPI(t *testing.T) {
	pdfPath := filepath.Join("..", "..", "examples", "2603.29199v1.pdf")
	if _, err := os.Stat(pdfPath); err != nil {
		t.Skip("test PDF not available")
	}

	tmpDir := t.TempDir()
	pages, err := ExtractPages(pdfPath, 0, tmpDir)
	if err != nil {
		t.Fatalf("ExtractPages with default DPI failed: %v", err)
	}
	if len(pages) == 0 {
		t.Fatal("expected at least one page")
	}
}

func TestExtractPages_NonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := ExtractPages("/nonexistent/file.pdf", 200, tmpDir)
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestExtractPagesJPEG(t *testing.T) {
	pdfPath := filepath.Join("..", "..", "examples", "2603.29199v1.pdf")
	if _, err := os.Stat(pdfPath); err != nil {
		t.Skip("test PDF not available")
	}

	tmpDir := t.TempDir()
	pages, err := ExtractPagesJPEG(pdfPath, 150, tmpDir, 80)
	if err != nil {
		t.Fatalf("ExtractPagesJPEG failed: %v", err)
	}

	if len(pages) == 0 {
		t.Fatal("expected at least one page")
	}

	// Verify JPEG files
	for _, page := range pages {
		if filepath.Ext(page) != ".jpg" {
			t.Errorf("expected .jpg extension, got %s", filepath.Ext(page))
		}
	}
}
