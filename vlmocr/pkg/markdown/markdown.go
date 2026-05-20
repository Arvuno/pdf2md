// Package markdown converts dots.mocr layout JSON output to Markdown format.
//
// Layout categories and their handling:
//   - Picture: omitted (or embedded as base64 image in full version)
//   - Formula: formatted as LaTeX ($$...$$)
//   - Table: output as HTML (raw)
//   - Page-header, Page-footer: optionally skipped
//   - All others (Text, Title, Section-header, etc.): output as-is
package markdown

import (
	"encoding/json"
	"fmt"
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

	return CellsToMarkdown(cells, opts), nil
}

// CellsToMarkdown converts a slice of LayoutCell to Markdown text.
func CellsToMarkdown(cells []LayoutCell, opts Options) string {
	var parts []string

	for _, cell := range cells {
		// Skip headers and footers if configured
		if opts.SkipHeadersFooters &&
			(cell.Category == "Page-header" || cell.Category == "Page-footer") {
			continue
		}

		text := strings.TrimSpace(cell.Text)

		switch cell.Category {
		case "Picture":
			// Pictures are omitted in the text-only markdown output
			continue
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
	for i, page := range pages {
		page = strings.TrimSpace(page)
		if page == "" {
			continue
		}
		if len(pages) > 1 {
			parts = append(parts, fmt.Sprintf("<!-- Page %d -->\n\n%s", i+1, page))
		} else {
			parts = append(parts, page)
		}
	}
	return strings.Join(parts, "\n\n---\n\n")
}
