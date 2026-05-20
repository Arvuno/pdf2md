package markdown

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConvert_ValidJSON(t *testing.T) {
	cells := `[
		{"bbox": [0,0,100,50], "category": "Title", "text": "My Title"},
		{"bbox": [0,60,100,100], "category": "Text", "text": "Some text."}
	]`

	md, err := Convert(cells, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(md, "My Title") {
		t.Errorf("expected 'My Title' in output, got: %s", md)
	}
	if !strings.Contains(md, "Some text.") {
		t.Errorf("expected 'Some text.' in output, got: %s", md)
	}
}

func TestConvert_FormulaHandling(t *testing.T) {
	cells := `[
		{"bbox": [0,0,100,50], "category": "Formula", "text": "E = mc^2"}
	]`

	md, err := Convert(cells, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(md, "$$") {
		t.Errorf("expected LaTeX $$ delimiters, got: %s", md)
	}
	if !strings.Contains(md, "E = mc^2") {
		t.Errorf("expected formula content, got: %s", md)
	}
}

func TestConvert_FormulaAlreadyWrapped(t *testing.T) {
	cells := `[
		{"bbox": [0,0,100,50], "category": "Formula", "text": "$$\\int_0^1 x dx = 0.5$$"}
	]`

	md, err := Convert(cells, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(md, "$$") {
		t.Errorf("expected $$ delimiters, got: %s", md)
	}
}

func TestConvert_FormulaSquareBrackets(t *testing.T) {
	cells := `[
		{"bbox": [0,0,100,50], "category": "Formula", "text": "\\[x^2 + y^2 = z^2\\]"}
	]`

	md, err := Convert(cells, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(md, "$$") {
		t.Errorf("expected $$ delimiters (converted from \\[\\]), got: %s", md)
	}
}

func TestConvert_TableAsHTML(t *testing.T) {
	tableHTML := `<table><tr><td>A</td><td>B</td></tr></table>`
	cells := `[
		{"bbox": [0,0,100,50], "category": "Table", "text": "` + tableHTML + `"}
	]`

	md, err := Convert(cells, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(md, "<table>") {
		t.Errorf("expected raw HTML table, got: %s", md)
	}
}

func TestConvert_PictureOmitted(t *testing.T) {
	cells := `[
		{"bbox": [0,0,100,50], "category": "Picture", "text": ""},
		{"bbox": [0,60,100,100], "category": "Text", "text": "After picture"}
	]`

	md, err := Convert(cells, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(md, "Picture") {
		t.Errorf("expected Picture to be omitted, got: %s", md)
	}
	if !strings.Contains(md, "After picture") {
		t.Errorf("expected text after picture, got: %s", md)
	}
}

func TestConvert_SkipHeadersFooters(t *testing.T) {
	cells := `[
		{"bbox": [0,0,100,30], "category": "Page-header", "text": "Header"},
		{"bbox": [0,50,100,100], "category": "Text", "text": "Content"},
		{"bbox": [0,900,100,950], "category": "Page-footer", "text": "Footer"}
	]`

	// Without skip
	md, _ := Convert(cells, Options{SkipHeadersFooters: false})
	if !strings.Contains(md, "Header") {
		t.Error("expected Header when not skipping")
	}
	if !strings.Contains(md, "Footer") {
		t.Error("expected Footer when not skipping")
	}

	// With skip
	md, _ = Convert(cells, Options{SkipHeadersFooters: true})
	if strings.Contains(md, "Header") {
		t.Errorf("expected Header to be skipped, got: %s", md)
	}
	if strings.Contains(md, "Footer") {
		t.Errorf("expected Footer to be skipped, got: %s", md)
	}
	if !strings.Contains(md, "Content") {
		t.Errorf("expected Content to remain, got: %s", md)
	}
}

func TestConvert_InvalidJSON(t *testing.T) {
	raw := "This is not JSON, just raw text from a failed model output."
	md, err := Convert(raw, Options{})

	// Should return the raw text with an error
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
	if md != raw {
		t.Errorf("expected raw text back, got: %s", md)
	}
}

func TestConvert_EmptyJSON(t *testing.T) {
	cells := `[]`
	md, err := Convert(cells, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if md != "" {
		t.Errorf("expected empty string for empty cells, got: %q", md)
	}
}

func TestCombinePages_SinglePage(t *testing.T) {
	result := CombinePages([]string{"Page content"})
	if result != "Page content" {
		t.Errorf("expected single page without markers, got: %q", result)
	}
}

func TestCombinePages_MultiplePages(t *testing.T) {
	pages := []string{"Page 1 content", "Page 2 content", "Page 3 content"}
	result := CombinePages(pages)

	if !strings.Contains(result, "Page 1 content") {
		t.Error("expected Page 1 content")
	}
	if !strings.Contains(result, "Page 2 content") {
		t.Error("expected Page 2 content")
	}
	if !strings.Contains(result, "Page 3 content") {
		t.Error("expected Page 3 content")
	}
	if !strings.Contains(result, "\n\n") {
		t.Error("expected double newline between pages")
	}
}

func TestCombinePages_EmptyPagesSkipped(t *testing.T) {
	pages := []string{"Page 1", "", "Page 3"}
	result := CombinePages(pages)

	if strings.Contains(result, "<!-- Page 2 -->") {
		t.Error("expected empty Page 2 to be skipped")
	}
}

func TestCellsToMarkdown_AllCategories(t *testing.T) {
	cells := []LayoutCell{
		{BBox: []int{0, 0, 100, 50}, Category: "Title", Text: "Title"},
		{BBox: []int{0, 50, 100, 100}, Category: "Text", Text: "Text"},
		{BBox: []int{0, 100, 100, 150}, Category: "Section-header", Text: "Header"},
		{BBox: []int{0, 150, 100, 200}, Category: "Caption", Text: "Caption"},
		{BBox: []int{0, 200, 100, 250}, Category: "Footnote", Text: "Footnote"},
		{BBox: []int{0, 250, 100, 300}, Category: "List-item", Text: "Item"},
		{BBox: []int{0, 300, 100, 350}, Category: "Formula", Text: "x^2"},
		{BBox: []int{0, 350, 100, 400}, Category: "Table", Text: "<table/>"},
		{BBox: []int{0, 400, 100, 450}, Category: "Picture", Text: ""},
	}

	md := CellsToMarkdown(cells, Options{})

	// Non-picture, non-formula, non-table items should appear
	for _, expected := range []string{"Title", "Text", "Header", "Caption", "Footnote", "Item"} {
		if !strings.Contains(md, expected) {
			t.Errorf("expected %q in output", expected)
		}
	}

	// Formula should be wrapped
	if !strings.Contains(md, "$$") {
		t.Error("expected $$ for formula")
	}

	// Table should be raw HTML
	if !strings.Contains(md, "<table/>") {
		t.Error("expected raw table HTML")
	}
}

func TestFormatFormula(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		{"plain", "x^2", "$$\nx^2\n$$"},
		{"already wrapped", "$$E=mc^2$$", "$$\nE=mc^2\n$$"},
		{"square brackets", "\\[a+b\\]", "$$\na+b\n$$"},
		{"inline dollar", "$x$", "$x$"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatFormula(tt.input)
			if got != tt.want {
				t.Errorf("formatFormula(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestConvert_TestdataFile(t *testing.T) {
	testdataDir := filepath.Join("..", "..", "testdata")
	data, err := os.ReadFile(filepath.Join(testdataDir, "sample_response.json"))
	if err != nil {
		t.Skipf("testdata not available: %v", err)
	}

	md, err := Convert(string(data), Options{SkipHeadersFooters: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should contain the title
	if !strings.Contains(md, "A Sample Document Title") {
		t.Error("expected title in output")
	}

	// Should contain text
	if !strings.Contains(md, "This is a paragraph") {
		t.Error("expected paragraph text")
	}

	// Should contain section header
	if !strings.Contains(md, "1. Introduction") {
		t.Error("expected section header")
	}

	// Should contain formula wrapped in $$
	if !strings.Contains(md, "$$") {
		t.Error("expected formula $$ delimiters")
	}

	// Should contain table HTML
	if !strings.Contains(md, "<table>") {
		t.Error("expected table HTML")
	}

	// Should contain caption
	if !strings.Contains(md, "Table 1: Sample table caption") {
		t.Error("expected caption")
	}

	// Should NOT contain Picture (omitted)
	if strings.Contains(md, "Picture") {
		t.Error("Picture should be omitted")
	}

	// Should contain list items
	if !strings.Contains(md, "First item in a list") {
		t.Error("expected list item")
	}

	// Should contain footnote
	if !strings.Contains(md, "This is a footnote") {
		t.Error("expected footnote")
	}

	// Should NOT contain header/footer (skipped)
	if strings.Contains(md, "Header Text") {
		t.Error("header should be skipped")
	}
	if strings.Contains(md, "Page 1") {
		t.Error("footer should be skipped")
	}
}
