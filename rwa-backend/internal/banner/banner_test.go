package banner

import (
	"bytes"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
)

func TestRenderFramesEveryLineAtOneWidth(t *testing.T) {
	var b bytes.Buffer
	Render(&b, "RWA Explorer · test", "a subtitle", []Section{
		{Title: "service", Rows: []Row{R("version", "dev"), Rf("host", "%s, pid %d", "h", 1)}},
		{Title: "empty"},
		{Title: "long", Rows: []Row{R("value", strings.Repeat("word ", 40)), R("lines", "one\ntwo")}},
	})
	lines := strings.Split(strings.TrimSuffix(b.String(), "\n"), "\n")
	width := utf8.RuneCountInString(lines[0])
	for _, l := range lines {
		assert.Equal(t, width, utf8.RuneCountInString(l), l)
	}
	assert.True(t, strings.HasPrefix(lines[0], "╭") && strings.HasPrefix(lines[len(lines)-1], "╰"))
	assert.NotContains(t, b.String(), "empty", "a section without rows is skipped")
	assert.Contains(t, b.String(), "├─ long ")
	assert.Contains(t, b.String(), "│ lines         one")
	assert.Contains(t, b.String(), "│               two")
	assert.LessOrEqual(t, width, maxWidth+4)
}
