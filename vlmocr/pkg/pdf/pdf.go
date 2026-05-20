// Package pdf provides PDF page extraction using go-fitz (MuPDF).
// No external dependencies required.
package pdf

import (
	"fmt"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"

	"github.com/gen2brain/go-fitz"
)

// PageCount returns the number of pages in a PDF file.
func PageCount(pdfPath string) (int, error) {
	doc, err := fitz.New(pdfPath)
	if err != nil {
		return 0, fmt.Errorf("opening PDF %s: %w", pdfPath, err)
	}
	defer doc.Close()
	return doc.NumPage(), nil
}

// ExtractPages converts PDF pages to PNG images.
// Returns a list of image file paths, one per page, in order.
func ExtractPages(pdfPath string, dpi int, outputDir string) ([]string, error) {
	if dpi <= 0 {
		dpi = 200
	}

	doc, err := fitz.New(pdfPath)
	if err != nil {
		return nil, fmt.Errorf("opening PDF %s: %w", pdfPath, err)
	}
	defer doc.Close()

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("creating output directory: %w", err)
	}

	numPages := doc.NumPage()
	pages := make([]string, 0, numPages)

	for i := 0; i < numPages; i++ {
		// Render page to image at specified DPI
		img, err := doc.ImageDPI(i, float64(dpi))
		if err != nil {
			return nil, fmt.Errorf("rendering page %d: %w", i+1, err)
		}

		pagePath := filepath.Join(outputDir, fmt.Sprintf("page-%d.png", i+1))
		f, err := os.Create(pagePath)
		if err != nil {
			return nil, fmt.Errorf("creating file for page %d: %w", i+1, err)
		}

		if err := png.Encode(f, img); err != nil {
			f.Close()
			return nil, fmt.Errorf("encoding page %d: %w", i+1, err)
		}
		f.Close()

		pages = append(pages, pagePath)
	}

	return pages, nil
}

// ExtractPagesJPEG converts PDF pages to JPEG images.
func ExtractPagesJPEG(pdfPath string, dpi int, outputDir string, quality int) ([]string, error) {
	if dpi <= 0 {
		dpi = 200
	}
	if quality <= 0 {
		quality = 90
	}

	doc, err := fitz.New(pdfPath)
	if err != nil {
		return nil, fmt.Errorf("opening PDF %s: %w", pdfPath, err)
	}
	defer doc.Close()

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("creating output directory: %w", err)
	}

	numPages := doc.NumPage()
	pages := make([]string, 0, numPages)

	for i := 0; i < numPages; i++ {
		img, err := doc.ImageDPI(i, float64(dpi))
		if err != nil {
			return nil, fmt.Errorf("rendering page %d: %w", i+1, err)
		}

		pagePath := filepath.Join(outputDir, fmt.Sprintf("page-%d.jpg", i+1))
		f, err := os.Create(pagePath)
		if err != nil {
			return nil, fmt.Errorf("creating file for page %d: %w", i+1, err)
		}

		if err := jpeg.Encode(f, img, &jpeg.Options{Quality: quality}); err != nil {
			f.Close()
			return nil, fmt.Errorf("encoding page %d: %w", i+1, err)
		}
		f.Close()

		pages = append(pages, pagePath)
	}

	return pages, nil
}
