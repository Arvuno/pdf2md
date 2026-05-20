package htmlmd

import (
	"strings"
	"testing"
)

func TestConvert_RemovesImgBbox(t *testing.T) {
	html := `<img data-bbox="100,200,300,400" src="test.png">Keep this`
	result := Convert(html)
	if strings.Contains(result, "img") {
		t.Errorf("expected img tag removed, got: %s", result)
	}
	if !strings.Contains(result, "Keep this") {
		t.Error("expected surrounding text preserved")
	}
}

func TestConvert_CodeBlock(t *testing.T) {
	html := `<div class="code"><pre>print("hello")</pre></div>`
	result := Convert(html)
	if !strings.Contains(result, "```code") {
		t.Errorf("expected ```code block, got: %s", result)
	}
	if !strings.Contains(result, `print("hello")`) {
		t.Errorf("expected code content preserved, got: %s", result)
	}
}

func TestConvert_TableDiv(t *testing.T) {
	html := `<div class="table"><table><tr><td>A</td><td>B</td></tr></table></div>`
	result := Convert(html)
	if !strings.Contains(result, "<table>") {
		t.Errorf("expected table HTML preserved, got: %s", result)
	}
	if strings.Contains(result, `<div class="table">`) {
		t.Error("expected div wrapper removed")
	}
}

func TestConvert_FormulaDiv(t *testing.T) {
	html := `<div class="formula">$$E=mc^2$$</div>`
	result := Convert(html)
	if !strings.Contains(result, "$$E=mc^2$$") {
		t.Errorf("expected formula preserved, got: %s", result)
	}
}

func TestConvert_ParagraphTag(t *testing.T) {
	html := `<p>First paragraph</p><p>Second paragraph</p>`
	result := Convert(html)
	if strings.Contains(result, "<p>") {
		t.Errorf("expected <p> tags removed, got: %s", result)
	}
	if !strings.Contains(result, "First paragraph") {
		t.Error("expected paragraph text preserved")
	}
}

func TestConvert_MultipleElements(t *testing.T) {
	html := `<p>Title</p><div class="table"><table><tr><td>Data</td></tr></table></div><div class="formula">x^2</div>`
	result := Convert(html)
	if !strings.Contains(result, "Title") {
		t.Error("expected Title")
	}
	if !strings.Contains(result, "<table>") {
		t.Error("expected table")
	}
	if !strings.Contains(result, "x^2") {
		t.Error("expected formula")
	}
}

func TestConvert_EmptyInput(t *testing.T) {
	result := Convert("")
	if result != "" {
		t.Errorf("expected empty output for empty input, got: %q", result)
	}
}

func TestConvert_PassthroughPlain(t *testing.T) {
	input := "Just plain text, no HTML"
	result := Convert(input)
	if result != input {
		t.Errorf("expected plain text passthrough, got: %q", result)
	}
}

func TestProcessCodeContent(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantSub string
	}{
		{"with pre tags", `<pre>code here</pre>`, "```code\ncode here\n```"},
		{"with backticks", "```code here```", "```code\ncode here\n```"},
		{"plain", "x = 1", "```code\nx = 1\n```"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processCodeContent(tt.input)
			if !strings.Contains(result, "```code") {
				t.Errorf("expected ```code, got: %s", result)
			}
		})
	}
}

func TestRemoveLinesStartingWith(t *testing.T) {
	input := "Z:skip this\nkeep this\nZ:skip too\nalso keep"
	result := removeLinesStartingWith(input, "Z:")
	if strings.Contains(result, "skip") {
		t.Errorf("expected Z: lines removed, got: %s", result)
	}
	if !strings.Contains(result, "keep this") {
		t.Error("expected non-Z: lines preserved")
	}
}
