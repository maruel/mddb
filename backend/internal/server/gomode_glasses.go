// Turns page Markdown into the plain text node_show_on_glasses sends to Go Mode glasses.

package server

import (
	"regexp"
	"strings"
	"unicode"
)

// glassesContentBytes bounds the Markdown read for the glasses to the window
// node_read returns. The client scrolls the text a line per swipe and shows at
// most 65536 characters of it.
const glassesContentBytes = mcpContentPageSize

// mdLink matches a Markdown link or image; the glasses show its text alone.
var mdLink = regexp.MustCompile(`!?\[([^\]]*)\]\([^)]*\)`)

// mdEmphasis lists the inline Markdown markers the glasses drop. Single "*"
// and "_" stay: they are as often part of a word as emphasis.
var mdEmphasis = strings.NewReplacer("**", "", "__", "", "~~", "", "`", "")

// glassesText turns Markdown into the plain text the glasses show: block
// markers and inline emphasis go, links and images keep their text, and runs
// of blank lines become one. Code blocks keep their lines. When cut, markdown
// is the start of a longer page: its last, partial line goes and the text ends
// with an ellipsis.
func glassesText(markdown string, cut bool) string {
	lines := strings.Split(markdown, "\n")
	if cut && len(lines) > 1 {
		lines = lines[:len(lines)-1]
	}
	var out []string
	fence := ""
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if fence != "" {
			if strings.HasPrefix(t, fence) {
				fence = ""
				continue
			}
			out = append(out, strings.TrimRightFunc(line, unicode.IsSpace))
			continue
		}
		if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			fence = t[:3]
			continue
		}
		if isTableDelimiter(t) {
			continue
		}
		t = glassesLine(t)
		if t == "" && (len(out) == 0 || out[len(out)-1] == "") {
			continue
		}
		out = append(out, t)
	}
	text := strings.TrimSpace(strings.Join(out, "\n"))
	if cut && text != "" {
		text += "…"
	}
	return text
}

// glassesLine is one trimmed Markdown line outside a code block as plain
// text; a rule becomes a blank line.
func glassesLine(t string) string {
	if isThematicBreak(t) {
		return ""
	}
	for strings.HasPrefix(t, ">") {
		t = strings.TrimSpace(t[1:])
	}
	if h := strings.TrimLeft(t, "#"); len(t)-len(h) <= 6 && h != t && (h == "" || h[0] == ' ') {
		t = strings.TrimSpace(h)
	}
	for _, bullet := range []string{"- ", "* ", "+ "} {
		if rest, ok := strings.CutPrefix(t, bullet); ok {
			switch {
			case strings.HasPrefix(rest, "[ ] "), strings.HasPrefix(rest, "[x] "), strings.HasPrefix(rest, "[X] "):
				t = "[" + strings.ToLower(rest[1:2]) + "]" + rest[3:]
			default:
				t = "- " + rest
			}
			break
		}
	}
	t = mdLink.ReplaceAllString(t, "$1")
	return strings.TrimSpace(mdEmphasis.Replace(t))
}

// isThematicBreak reports whether t is a Markdown rule: three or more of the
// same "-", "*", or "_", with optional spaces.
func isThematicBreak(t string) bool {
	s := strings.ReplaceAll(t, " ", "")
	return len(s) >= 3 && strings.Trim(s, s[:1]) == "" && strings.ContainsAny(s[:1], "-*_")
}

// isTableDelimiter reports whether t is the delimiter row of a Markdown table,
// such as "|---|:--:|".
func isTableDelimiter(t string) bool {
	return strings.HasPrefix(t, "|") && strings.Trim(t, "|-: ") == "" && strings.Contains(t, "-")
}
