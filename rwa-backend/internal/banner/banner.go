// Package banner renders the startup summary of a service: a framed block of titled sections with key/value rows,
// printed once before the first log line so the operator sees what the process listens on, connects to and uses.
package banner

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

const (
	minWidth = 64  // inner width at least
	maxWidth = 108 // longer values wrap
	keyWidth = 12
)

// Row is one line of a section: a short key and its value. A value may hold "\n" for explicit line breaks; long
// lines wrap at spaces.
type Row struct {
	Key   string
	Value string
}

// Section is a titled group of rows.
type Section struct {
	Title string
	Rows  []Row
}

// R is a shorthand for a Row.
func R(key, value string) Row { return Row{key, value} }

// Rf is a Row with a formatted value.
func Rf(key, format string, args ...any) Row { return Row{key, fmt.Sprintf(format, args...)} }

// Render writes the framed banner: title and subtitle in the header, then every non-empty section.
func Render(w io.Writer, title, subtitle string, sections []Section) {
	type line struct {
		text string
		rule string // non-empty: a section rule with this title
	}
	valueWidth := maxWidth - keyWidth - 2
	var lines []line
	lines = append(lines, line{text: title})
	if subtitle != "" {
		for _, l := range wrap(subtitle, maxWidth) {
			lines = append(lines, line{text: l})
		}
	}
	for _, s := range sections {
		if len(s.Rows) == 0 {
			continue
		}
		lines = append(lines, line{rule: s.Title})
		for _, r := range s.Rows {
			for i, v := range wrap(r.Value, valueWidth) {
				key := ""
				if i == 0 {
					key = r.Key
				}
				lines = append(lines, line{text: pad(key, keyWidth) + "  " + v})
			}
		}
	}

	width := minWidth
	for _, l := range lines {
		width = max(width, runes(l.text), runes(l.rule)+4)
	}

	var b strings.Builder
	b.WriteString("╭" + strings.Repeat("─", width+2) + "╮\n")
	for _, l := range lines {
		if l.rule != "" {
			head := "├─ " + l.rule + " "
			b.WriteString(head + strings.Repeat("─", width+3-runes(head)) + "┤\n")
			continue
		}
		b.WriteString("│ " + pad(l.text, width) + " │\n")
	}
	b.WriteString("╰" + strings.Repeat("─", width+2) + "╯\n")
	_, _ = io.WriteString(w, b.String())
}

func runes(s string) int { return utf8.RuneCountInString(s) }

func pad(s string, n int) string {
	if d := n - runes(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return s
}

// wrap splits s at "\n", then wraps every line at spaces so it fits in width runes (a single longer word stays whole).
func wrap(s string, width int) []string {
	var out []string
	for _, para := range strings.Split(s, "\n") {
		words := strings.Fields(para)
		if len(words) == 0 {
			out = append(out, "")
			continue
		}
		cur := words[0]
		for _, w := range words[1:] {
			if runes(cur)+1+runes(w) > width {
				out = append(out, cur)
				cur = "  " + w // continuation of a wrapped line
				continue
			}
			cur += " " + w
		}
		out = append(out, cur)
	}
	return out
}
