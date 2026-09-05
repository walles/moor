package internal

import (
	"testing"

	"gotest.tools/v3/assert"

	"github.com/walles/moor/v2/internal/textstyles"
	"github.com/walles/twin"

	log "github.com/sirupsen/logrus"
)

func tokenize(input string) []textstyles.CellWithMetadata {
	return textstyles.StyledRunesFromString(twin.StyleDefault, input, nil, 0).StyledRunes
}

func rowsToString(cellLines []textstyles.CellWithMetadataSlice) string {
	returnMe := ""
	for _, cellLine := range cellLines {
		lineString := ""
		for _, cell := range cellLine {
			lineString += string(cell.Rune)
		}

		if len(returnMe) > 0 {
			returnMe += "\n"
		}
		returnMe += "<" + lineString + ">"
	}

	return returnMe
}

func assertWrap(t *testing.T, input string, widthInScreenCells int, wrappedLines ...string) {
	// Temporarily disable the minimum wrap width for tests
	originalMinWrapWidth := minWrapWidth
	minWrapWidth = 0
	t.Cleanup(func() { minWrapWidth = originalMinWrapWidth })

	toWrap := tokenize(input)
	wrapped := wrapLine(widthInScreenCells, toWrap)
	actual := make([]textstyles.CellWithMetadataSlice, len(wrapped))
	for i, w := range wrapped {
		actual[i] = w.StyledRunes
	}

	expected := []textstyles.CellWithMetadataSlice{}
	for _, wrappedLine := range wrappedLines {
		expected = append(expected, tokenize(wrappedLine))
	}

	equal := true
	if len(actual) != len(expected) {
		equal = false
	} else {
		for i := range actual {
			if !actual[i].Equal(expected[i]) {
				equal = false
				break
			}
		}
	}
	if equal {
		return
	}

	t.Errorf("When wrapping <%s> at cell count %d:\n--Expected--\n%s\n\n--Actual--\n%s",
		input, widthInScreenCells, rowsToString(expected), rowsToString(actual))
}

func TestEnoughRoomNoWrapping(t *testing.T) {
	assertWrap(t, "This is a test", 20, "This is a test")
}

func TestWrapBlank(t *testing.T) {
	assertWrap(t, "    ", 4, "")
	assertWrap(t, "    ", 2, "")

	assertWrap(t, "", 20, "")
}

func TestWordLongerThanLine(t *testing.T) {
	assertWrap(t, "intermediary", 6, "interm", "ediary")
}

func TestLeadingSpaceNoWrap(t *testing.T) {
	assertWrap(t, " abc", 20, " abc")
}

func TestLeadingSpaceWithWrap(t *testing.T) {
	assertWrap(t, " abc", 2, " a", " b", " c")
}

func TestLeadingWrappedSpace(t *testing.T) {
	assertWrap(t, "ab cd", 2, "ab", "cd")
}

func TestWordWrap(t *testing.T) {
	assertWrap(t, "abc 123", 8, "abc 123")
	assertWrap(t, "abc 123", 7, "abc 123")
	assertWrap(t, "abc 123", 6, "abc", "123")
	assertWrap(t, "abc 123", 5, "abc", "123")
	assertWrap(t, "abc 123", 4, "abc", "123")
	assertWrap(t, "abc 123", 3, "abc", "123")
	assertWrap(t, "abc 123", 2, "ab", "c", "12", "3")

	assertWrap(t, "here's the last line", 10, "here's the", "last line")
}

func TestWordWrapUrl(t *testing.T) {
	assertWrap(t, "http://apa/bepa/", 17, "http://apa/bepa/")
	assertWrap(t, "http://apa/bepa/", 16, "http://apa/bepa/")
	assertWrap(t, "http://apa/bepa/", 15, "http://apa/", "bepa/")
	assertWrap(t, "http://apa/bepa/", 14, "http://apa/", "bepa/")
	assertWrap(t, "http://apa/bepa/", 13, "http://apa/", "bepa/")
	assertWrap(t, "http://apa/bepa/", 12, "http://apa/", "bepa/")
	assertWrap(t, "http://apa/bepa/", 11, "http://apa/", "bepa/")
	assertWrap(t, "http://apa/bepa/", 10, "http://apa", "/bepa/")
	assertWrap(t, "http://apa/bepa/", 9, "http://ap", "a/bepa/")
	assertWrap(t, "http://apa/bepa/", 8, "http://a", "pa/bepa/")
	assertWrap(t, "http://apa/bepa/", 7, "http://", "apa/", "bepa/")
	assertWrap(t, "http://apa/bepa/", 6, "http:/", "/apa/", "bepa/")
	assertWrap(t, "http://apa/bepa/", 5, "http:", "//apa", "/bepa", "/")
	assertWrap(t, "http://apa/bepa/", 4, "http", "://a", "pa/", "bepa", "/")
	assertWrap(t, "http://apa/bepa/", 3, "htt", "p:/", "/ap", "a/", "bep", "a/")
}

func TestWordWrapMarkdownLink(t *testing.T) {
	assertWrap(t, "[something](http://apa/bepa)", 13, "[something]", "(http://apa/", "bepa)")
	assertWrap(t, "[something](http://apa/bepa)", 12, "[something]", "(http://apa/", "bepa)")
	assertWrap(t, "[something](http://apa/bepa)", 11, "[something]", "(http://apa", "/bepa)")

	// This doesn't look great, room for tuning!
	assertWrap(t, "[something](http://apa/bepa)", 10, "[something", "]", "(http://ap", "a/bepa)")
}

func TestWordWrapWideChars(t *testing.T) {
	// The width is in cells, and there are wide chars in here using multiple cells.
	assertWrap(t, "x上午y", 6, "x上午y")
	assertWrap(t, "x上午y", 5, "x上午", "y")
	assertWrap(t, "x上午y", 4, "x上", "午y")
	assertWrap(t, "x上午y", 3, "x上", "午y")
	assertWrap(t, "x上午y", 2, "x", "上", "午", "y")
}

func TestGetWrapCountWideChars(t *testing.T) {
	line := tokenize("x上午y")
	assert.Equal(t, getWrapCount(line, 5), 3)
	assert.Equal(t, getWrapCount(line, 4), 2)
	assert.Equal(t, getWrapCount(line, 3), 2)
	assert.Equal(t, getWrapCount(line, 2), 1)
	assert.Equal(t, getWrapCount(line, 1), 1)
}

func TestGetHangingIndentWidth(t *testing.T) {
	assert.Equal(t, 0, getHangingIndentWidth(tokenize("")))
	assert.Equal(t, 0, getHangingIndentWidth(tokenize("hello")))
	assert.Equal(t, 3, getHangingIndentWidth(tokenize("   hello")))
	assert.Equal(t, 6, getHangingIndentWidth(tokenize("   1. numbered list")))

	// Can be found on man pages
	assert.Equal(t, 7, getHangingIndentWidth(tokenize("   -D  after the double (more-than-one) space")))
}

func BenchmarkWrapLine(b *testing.B) {
	log.SetLevel(log.WarnLevel) // Stop info logs from polluting benchmark output

	words := "Here are some words of different lengths, some of which are very long, and some of which are short. "
	lineLen := 60_000
	line := ""
	for len(line) < lineLen {
		line += words
	}
	line = line[:lineLen]

	styledRunes := tokenize(line)

	for b.Loop() {
		_ = wrapLine(73, styledRunes)
	}
}
