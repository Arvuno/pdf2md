// Package markdown converts dots.mocr layout JSON output to Markdown format.
//
// Layout categories and their handling:
//   - Picture: cropped from page image, saved as PNG, referenced via relative path
//   - Formula: formatted as LaTeX ($$...$$)
//   - Table: output as HTML (raw)
//   - Page-header, Page-footer: optionally skipped
//   - All others (Text, Title, Section-header, etc.): output as-is
package markdown

import (
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"
)

// LayoutCell represents a single layout element from the dots.mocr model output.
type LayoutCell struct {
	BBox     []int  `json:"bbox"`
	Category string `json:"category"`
	Text     string `json:"text,omitempty"`
}

// Options controls the markdown conversion behavior.
type Options struct {
	// SkipHeadersFooters removes Page-header and Page-footer elements.
	SkipHeadersFooters bool
	// ImagePath is the source page image used to crop Picture blocks.
	ImagePath string
	// CropOutputDir is where cropped Picture PNGs are saved.
	CropOutputDir string
}

// Convert converts a JSON layout response string to Markdown.
// The response is expected to be a JSON array of LayoutCell objects.
// If JSON parsing fails, the raw response is returned as-is.
func Convert(response string, opts Options) (string, error) {
	response = strings.TrimSpace(response)

	// Try to parse as JSON array of layout cells
	var cells []LayoutCell
	if err := json.Unmarshal([]byte(response), &cells); err != nil {
		// If JSON parsing fails, return the raw response as filtered text
		return response, fmt.Errorf("failed to parse layout JSON (returning raw): %w", err)
	}

	md := CellsToMarkdown(cells, opts)
	return md, nil
}

// CellsToMarkdown converts a slice of LayoutCell to Markdown text.
func CellsToMarkdown(cells []LayoutCell, opts Options) string {
	var parts []string

	for i, cell := range cells {
		// Skip headers and footers if configured
		if opts.SkipHeadersFooters &&
			(cell.Category == "Page-header" || cell.Category == "Page-footer") {
			continue
		}

		text := strings.TrimSpace(cell.Text)

		switch cell.Category {
		case "Picture":
			imgRef := cropAndSavePicture(cell, i, opts)
			if imgRef != "" {
				parts = append(parts, imgRef)
			}
		case "Formula":
			parts = append(parts, formatFormula(text))
		case "Table":
			// Tables are output as raw HTML
			if text != "" {
				parts = append(parts, text)
			}
		default:
			// Text, Title, Section-header, Caption, Footnote, List-item, etc.
			if text != "" {
				parts = append(parts, text)
			}
		}
	}

	return strings.Join(parts, "\n\n")
}

// cropAndSavePicture crops a picture region from the source image and returns
// a Markdown image reference with a relative path.
func cropAndSavePicture(cell LayoutCell, index int, opts Options) string {
	if opts.ImagePath == "" || opts.CropOutputDir == "" {
		return ""
	}
	if len(cell.BBox) != 4 {
		return ""
	}

	src, err := openImage(opts.ImagePath)
	if err != nil {
		return ""
	}

	bounds := src.Bounds()
	x1, y1 := clampCoord(cell.BBox[0], bounds.Min.X, bounds.Max.X), clampCoord(cell.BBox[1], bounds.Min.Y, bounds.Max.Y)
	x2, y2 := clampCoord(cell.BBox[2], bounds.Min.X, bounds.Max.X), clampCoord(cell.BBox[3], bounds.Min.Y, bounds.Max.Y)
	if x2 < x1 {
		x1, x2 = x2, x1
	}
	if y2 < y1 {
		y1, y2 = y2, y1
	}
	if x2 <= x1 || y2 <= y1 {
		return ""
	}

	crop := image.NewRGBA(image.Rect(0, 0, x2-x1, y2-y1))
	for y := 0; y < y2-y1; y++ {
		for x := 0; x < x2-x1; x++ {
			crop.Set(x, y, src.At(x1+x, y1+y))
		}
	}

	if err := os.MkdirAll(opts.CropOutputDir, 0755); err != nil {
		return ""
	}

	name := fmt.Sprintf("picture-%03d.png", index+1)
	path := filepath.Join(opts.CropOutputDir, name)
	if err := writePNG(path, crop); err != nil {
		return ""
	}

	// Use a relative path from the markdown file's perspective:
	// Markdown is typically in the output dir, crops are in a subdir.
	// The path is already relative to CWD; make it relative to the output dir's parent.
	// We use filepath.ToSlash for consistent separators.
	rel := filepath.ToSlash(filepath.Base(opts.CropOutputDir) + "/" + name)
	return fmt.Sprintf("![Picture](%s)", rel)
}

func openImage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	return img, err
}

func clampCoord(v, low, high int) int {
	if v < low {
		return low
	}
	if v > high {
		return high
	}
	return v
}

func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// formatFormula wraps formula text in LaTeX display math delimiters.
func formatFormula(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}

	// Already wrapped in $$
	if strings.HasPrefix(text, "$$") && strings.HasSuffix(text, "$$") {
		inner := strings.TrimSpace(text[2 : len(text)-2])
		if !strings.Contains(inner, "$") {
			return fmt.Sprintf("$$\n%s\n$$", inner)
		}
		return text
	}

	// Wrapped in \[...\]
	if strings.HasPrefix(text, "\\[") && strings.HasSuffix(text, "\\]") {
		inner := strings.TrimSpace(text[2 : len(text)-2])
		return fmt.Sprintf("$$\n%s\n$$", inner)
	}

	// Inline formula $...$
	if strings.Count(text, "$") >= 2 {
		return text
	}

	// Clean LaTeX preamble if present
	if strings.Contains(text, "usepackage") {
		text = cleanLatexPreamble(text)
	}

	// Remove backtick wrapping
	if len(text) >= 2 && text[0] == '`' && text[len(text)-1] == '`' {
		text = text[1 : len(text)-1]
	}

	return fmt.Sprintf("$$\n%s\n$$", text)
}

// cleanLatexPreamble removes common LaTeX preamble commands.
func cleanLatexPreamble(text string) string {
	lines := strings.Split(text, "\n")
	var cleaned []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "\\documentclass") ||
			strings.HasPrefix(trimmed, "\\usepackage") ||
			trimmed == "\\begin{document}" ||
			trimmed == "\\end{document}" {
			continue
		}
		cleaned = append(cleaned, line)
	}
	return strings.TrimSpace(strings.Join(cleaned, "\n"))
}

// CombinePages merges multiple page markdown results into a single document.
func CombinePages(pages []string) string {
	var parts []string
	for _, page := range pages {
		page = strings.TrimSpace(page)
		if page == "" {
			continue
		}
		parts = append(parts, page)
	}
	return strings.Join(parts, "\n\n")
}
