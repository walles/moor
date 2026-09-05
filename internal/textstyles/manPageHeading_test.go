package textstyles

import (
	"testing"

	"github.com/walles/twin"
	"gotest.tools/v3/assert"
)

func isManPageHeading(s string) bool {
	return parseManPageHeading(s, func(_ CellWithMetadata) {})
}

func TestIsManPageHeading(t *testing.T) {
	assert.Assert(t, !isManPageHeading(""))
	assert.Assert(t, !isManPageHeading("A"), "Incomplete sequence")
	assert.Assert(t, !isManPageHeading("A\b"), "Incomplete sequence")

	assert.Assert(t, isManPageHeading("A\bA"))
	assert.Assert(t, isManPageHeading("A\bA B\bB"), "Whitespace can be not-bold")

	assert.Assert(t, !isManPageHeading("A\bC"), "Different first and last char")
	assert.Assert(t, !isManPageHeading("a\ba"), "Not ALL CAPS")

	assert.Assert(t, !isManPageHeading("A\bAX"), "Incomplete sequence")

	assert.Assert(t, !isManPageHeading(" \b "), "Headings do not start with space")
}

func TestManPageHeadingFromString_NotBoldSpace(t *testing.T) {
	// Set a marker style we can recognize and test for
	ManPageHeading = twin.StyleDefault.WithForeground(twin.NewColor16(2))

	result := manPageHeadingFromString("A\bA B\bB")

	assert.Assert(t, result != nil)
	assert.Equal(t, len(result.StyledRunes), 3)
	assert.Equal(t, result.StyledRunes[0], CellWithMetadata{Rune: 'A', Style: ManPageHeading})
	assert.Equal(t, result.StyledRunes[1], CellWithMetadata{Rune: ' ', Style: ManPageHeading})
	assert.Equal(t, result.StyledRunes[2], CellWithMetadata{Rune: 'B', Style: ManPageHeading})
}

func TestManPageHeadingFromString_WithBoldSpace(t *testing.T) {
	// Set a marker style we can recognize and test for
	ManPageHeading = twin.StyleDefault.WithForeground(twin.NewColor16(2))

	result := manPageHeadingFromString("A\bA \b B\bB")

	assert.Assert(t, result != nil)
	assert.Equal(t, len(result.StyledRunes), 3)
	assert.Equal(t, result.StyledRunes[0], CellWithMetadata{Rune: 'A', Style: ManPageHeading})
	assert.Equal(t, result.StyledRunes[1], CellWithMetadata{Rune: ' ', Style: ManPageHeading})
	assert.Equal(t, result.StyledRunes[2], CellWithMetadata{Rune: 'B', Style: ManPageHeading})
}
