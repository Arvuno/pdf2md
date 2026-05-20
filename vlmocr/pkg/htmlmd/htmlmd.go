// Package htmlmd converts Logics-Parsing HTML output to clean Markdown.
// This is a Go port of the Python qwenvl_cast_html_tag() function from
// https://github.com/alibaba/Logics-Parsing/blob/main/inference_v2.py
package htmlmd

import (
	"regexp"
	"strings"
)

var (
	reImgBbox = regexp.MustCompile(`(?i)<img\b[^>]*\bdata-bbox\s*=\s*"?\d+,\d+,\d+,\d+"?[^>]*\/?>`)
	reDivCode = regexp.MustCompile(`(?is)<div\b[^>]*class="code"[^>]*>(.*?)</div>`)
	reDivPseudo = regexp.MustCompile(`(?is)<div\b[^>]*class="pseudocode"[^>]*>(.*?)</div>`)
	reDivClass = regexp.MustCompile(`(?is)\s*<div\b[^>]*class="([^"]*)"[^>]*>(.*?)</div>\s*`)
	reP = regexp.MustCompile(`(?is)<p\b[^>]*>(.*?)</p>`)
	rePreOpen = regexp.MustCompile(`(?i)^\s*<pre[^>]*>`)
	rePreClose = regexp.MustCompile(`(?i)</pre>\s*$`)
	reCodeOpen = regexp.MustCompile(`(?i)^\s*<code[^>]*>`)
	reCodeClose = regexp.MustCompile(`(?i)</code>\s*$`)
	reMathPattern = regexp.MustCompile(`(\$\$.*?\$\$|\$[^$]+\$)`)
	reMermaidClean = regexp.MustCompile(`(?im)^\s*(click|style|linkStyle|stroke|classDef|class)\b.*$`)
	reMermaidIcon = regexp.MustCompile(`(?im)^\s*(%%|::icon).*$`)
)

// Convert converts Logics-Parsing HTML output to Markdown.
func Convert(html string) string {
	output := html

	// 1. Remove <img> tags with data-bbox
	output = reImgBbox.ReplaceAllString(output, "")

	// 2. Process <div class="code"> → ```code blocks
	output = reDivCode.ReplaceAllStringFunc(output, func(match string) string {
		content := reDivCode.FindStringSubmatch(match)
		if len(content) < 2 {
			return match
		}
		return "\n\n" + processCodeContent(content[1]) + "\n\n"
	})

	// 3. Process <div class="pseudocode"> → ``` blocks
	output = reDivPseudo.ReplaceAllStringFunc(output, func(match string) string {
		content := reDivPseudo.FindStringSubmatch(match)
		if len(content) < 2 {
			return match
		}
		return "\n\n" + processPseudocodeContent(content[1]) + "\n\n"
	})

	// 4. Strip other div classes
	otherClasses := []string{"image", "chemistry", "table", "formula", "image caption", "table caption"}
	for _, cls := range otherClasses {
		output = stripDivClass(cls, output)
	}

	// 5. <p> → newlines
	output = reP.ReplaceAllString(output, "\n\n$1\n\n")

	// 6. Clean up
	output = strings.ReplaceAll(output, " </td>", "</td>")

	return output
}

func processCodeContent(content string) string {
	content = strings.ReplaceAll(content, "```", "")
	content = rePreOpen.ReplaceAllString(content, "")
	content = rePreClose.ReplaceAllString(content, "")
	content = reCodeOpen.ReplaceAllString(content, "")
	content = reCodeClose.ReplaceAllString(content, "")
	return "```code\n" + strings.TrimSpace(content) + "\n```"
}

func processPseudocodeContent(content string) string {
	content = strings.ReplaceAll(content, "```", "")
	content = rePreOpen.ReplaceAllString(content, "")
	content = rePreClose.ReplaceAllString(content, "")
	content = reCodeOpen.ReplaceAllString(content, "")
	content = reCodeClose.ReplaceAllString(content, "")

	// Protect LaTeX formulas
	var mathBlocks []string
	protected := reMathPattern.ReplaceAllStringFunc(content, func(match string) string {
		mathBlocks = append(mathBlocks, match)
		return "___MATH_ID_" + itoa(len(mathBlocks)-1) + "___"
	})

	// Convert spaces/tabs/newlines to HTML
	protected = strings.ReplaceAll(protected, " ", "&nbsp;")
	protected = strings.ReplaceAll(protected, "\t", "&nbsp;&nbsp;&nbsp;&nbsp;")
	protected = strings.ReplaceAll(protected, "\n", "<br>")

	// Restore LaTeX
	final := protected
	for i, original := range mathBlocks {
		final = strings.ReplaceAll(final, "___MATH_ID_"+itoa(i)+"___", original)
	}

	return "___\n<br>" + strings.TrimSpace(final) + "<br>\n___"
}

func stripDivClass(className, text string) string {
	re := regexp.MustCompile(`(?is)\s*<div\b[^>]*class="` + regexp.QuoteMeta(className) + `"[^>]*>(.*?)</div>\s*`)
	return re.ReplaceAllStringFunc(text, func(match string) string {
		parts := re.FindStringSubmatch(match)
		if len(parts) < 2 {
			return match
		}
		content := parts[1]

		switch className {
		case "chart":
			content = reMermaidClean.ReplaceAllString(content, "")
			content = reMermaidIcon.ReplaceAllString(content, "")
			content = strings.TrimSpace(content)
			if strings.HasPrefix(content, "mermaid") {
				content = "```" + content
			} else if strings.HasPrefix(content, "```mermaid") {
				// already formatted
			} else {
				content = "```mermaid\n" + content
			}
			if !strings.HasSuffix(content, "```") {
				content += "\n```"
			}

		case "music":
			content = removeLinesStartingWith(content, "Z:")
			content = strings.TrimSpace(content)
			if strings.HasPrefix(content, "abc") {
				content = "```" + content
			} else if strings.HasPrefix(content, "```abc") {
				// already formatted
			} else {
				content = "```abc\n" + content
			}
			if !strings.HasSuffix(content, "```") {
				content += "\n```"
			}
		}

		return "\n\n" + content + "\n\n"
	})
}

func removeLinesStartingWith(text, prefix string) string {
	lines := strings.Split(text, "\n")
	var filtered []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, prefix) {
			continue
		}
		filtered = append(filtered, line)
	}
	return strings.Join(filtered, "\n")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	result := ""
	negative := false
	if n < 0 {
		negative = true
		n = -n
	}
	for n > 0 {
		result = string(rune('0'+n%10)) + result
		n /= 10
	}
	if negative {
		result = "-" + result
	}
	return result
}
