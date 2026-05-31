package internal

import (
	"strings"
	"testing"

	"github.com/walles/moor/v2/internal/reader"
	"github.com/walles/moor/v2/twin"
	"gotest.tools/v3/assert"
)

func TestSwitchingToSmallerFileResetsScrollPosition(t *testing.T) {
	largeReader := reader.NewFromTextForTesting("large", strings.Repeat("large\n", 100))
	smallReader := reader.NewFromTextForTesting("small", "small\nfile")

	pager := NewPager(largeReader, smallReader)
	pager.screen = twin.NewFakeScreen(80, 10)
	pager.ShowLineNumbers = false
	pager.showLineNumbers = false

	pager.scrollToEnd()
	assert.Assert(t, pager.lineIndex().Index() > 0)

	pager.nextFile()
	pager.filteringReader.SetBackingReader(pager.readers[pager.currentReader])

	rendered := pager.renderLines()
	assert.Equal(t, renderedToString(rendered.lines[0].cells), "small")
	assert.Equal(t, pager.lineIndex().Index(), 0)
}
