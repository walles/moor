package internal

import (
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

// When filtering, the editor should be pointed at the middle line's line number
// in the file, not at its position among the filtered lines.
func TestMiddleVisibleLineWhenFiltering(t *testing.T) {
	pager := newFileSwitchingTestPager(t)
	pager.filter.For("a2")

	rendered := pager.renderLines().lines
	middle := strings.TrimRight(renderedToString(rendered[len(rendered)/2].cells), " ")
	assert.Equal(t, middle, "a23") // Filtered lines are a2, a20, a21, a22, a23, ...

	middleLine := middleVisibleLine(pager)
	assert.Assert(t, middleLine != nil)

	// "a23" is the 24th line of the file
	assert.Equal(t, middleLine.AsOneBased(), 24)
}
