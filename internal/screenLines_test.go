package internal

import (
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/walles/moor/v2/internal/linemetadata"
	"github.com/walles/moor/v2/internal/reader"
	"github.com/walles/moor/v2/internal/search"
	"github.com/walles/moor/v2/internal/textstyles"
	"github.com/walles/moor/v2/twin"
	"gotest.tools/v3/assert"

	log "github.com/sirupsen/logrus"
)

// NOTE: You can find related tests in pager_test.go.

// Converts a cell row to a plain string and removes trailing whitespace.
func renderedToString(row []textstyles.CellWithMetadata) string {
	rowString := ""
	for _, cell := range row {
		rowString += string(cell.Rune)
	}

	return strings.TrimRight(rowString, " ")
}

func testHorizontalCropping(t *testing.T, contents string, firstVisibleColumn int, lastVisibleColumn int, expected string) {
	log.SetLevel(log.WarnLevel) // Stop info logs from polluting benchmark output

	reader := reader.NewFromTextForTesting("testHorizontalCropping", contents)
	assert.NilError(t, reader.Wait())

	pager := NewPager(reader)
	pager.ShowLineNumbers = false
	pager.showLineNumbers = false

	pager.screen = twin.NewFakeScreen(1+lastVisibleColumn-firstVisibleColumn, 99)
	pager.leftColumnZeroBased = firstVisibleColumn
	pager.scrollPosition = newScrollPosition("testHorizontalCropping")

	numberedLine := reader.GetLine(linemetadata.IndexFromZeroBased(0))
	assert.Assert(t, numberedLine != nil)

	screenLine := pager.renderLine(*numberedLine, pager.getLineNumberPrefixLength(numberedLine.Number), true)
	assert.Equal(t, renderedToString(screenLine[0].cells), expected)
}

func TestCreateScreenLine(t *testing.T) {
	testHorizontalCropping(t, "abc", 0, 10, "abc")
}

func TestCreateScreenLineCanScrollLeft(t *testing.T) {
	testHorizontalCropping(t, "abc", 1, 10, "<c")
}

func TestCreateScreenLineCanScrollRight(t *testing.T) {
	testHorizontalCropping(t, "abc", 0, 1, "a>")
}

func TestCreateScreenLineCanAlmostScrollRight(t *testing.T) {
	testHorizontalCropping(t, "abc", 0, 2, "abc")
}

func TestCreateScreenLineCanScrollBoth(t *testing.T) {
	testHorizontalCropping(t, "abcde", 1, 3, "<c>")
}

func TestCreateScreenLineCanAlmostScrollBoth(t *testing.T) {
	testHorizontalCropping(t, "abcd", 1, 3, "<cd")
}

func TestCreateScreenLineChopWideCharLeft(t *testing.T) {
	testHorizontalCropping(t, "上午下", 0, 10, "上午下")
	testHorizontalCropping(t, "上午下", 1, 10, "<午下")
	testHorizontalCropping(t, "上午下", 2, 10, "< 下")
	testHorizontalCropping(t, "上午下", 3, 10, "<下")
	testHorizontalCropping(t, "上午下", 4, 10, "<")
	testHorizontalCropping(t, "上午下", 5, 10, "<")
	testHorizontalCropping(t, "上午下", 6, 10, "<")
	testHorizontalCropping(t, "上午下", 7, 10, "<")
}

func TestCreateScreenLineChopWideCharRight(t *testing.T) {
	testHorizontalCropping(t, "上午下", 0, 6, "上午下")
	testHorizontalCropping(t, "上午下", 0, 5, "上午下")
	testHorizontalCropping(t, "上午下", 0, 4, "上午>")
	testHorizontalCropping(t, "上午下", 0, 3, "上 >")
	testHorizontalCropping(t, "上午下", 0, 2, "上>")
	testHorizontalCropping(t, "上午下", 0, 1, " >")
}

func TestEmpty(t *testing.T) {
	pager := Pager{
		screen: twin.NewFakeScreen(99, 10),

		// No lines available
		readers: []*reader.ReaderImpl{reader.NewFromTextForTesting("test", "")},

		scrollPosition: newScrollPosition("TestEmpty"),
	}
	pager.filteringReader = FilteringReader{
		BackingReader: pager.readers[pager.currentReader],
		Filter:        &pager.filter,
	}

	rendered := pager.renderLines()
	assert.Equal(t, len(rendered.lines), 0)
	assert.Equal(t, "test", rendered.filenameText)
	assert.Equal(t, ": <empty>", rendered.statusText)
	assert.Assert(t, pager.lineIndex() == nil)
}

// Repro case for a search bug discovered in v1.9.8.
func TestSearchHighlight(t *testing.T) {
	numberedLine := reader.NewFromTextForTesting("TestSearchHighlight", "x\"\"x").GetLine(linemetadata.Index{})
	pager := Pager{
		screen: twin.NewFakeScreen(100, 10),
		search: search.For("\""),
	}

	rendered := pager.renderLine(*numberedLine, pager.getLineNumberPrefixLength(numberedLine.Number), true)
	assert.DeepEqual(t, []renderedLine{
		{
			inputLineIndex:    linemetadata.Index{},
			wrapIndex:         0,
			containsSearchHit: true,
			cells: []textstyles.CellWithMetadata{
				{Rune: 'x', Style: twin.StyleDefault},
				{Rune: '"', Style: twin.StyleDefault.WithAttr(twin.AttrReverse), IsSearchHit: true, StartsSearchHit: true},
				{Rune: '"', Style: twin.StyleDefault.WithAttr(twin.AttrReverse), IsSearchHit: true, StartsSearchHit: false},
				{Rune: 'x', Style: twin.StyleDefault},
			},
		},
	}, rendered,
		cmp.AllowUnexported(twin.Style{}),
		cmp.AllowUnexported(renderedLine{}),
		cmp.AllowUnexported(linemetadata.Number{}),
		cmp.AllowUnexported(linemetadata.Index{}),
	)
}

// Make sure that a search hit spanning the screen edge gets highlighted
func TestSearchHighlightTruncated(t *testing.T) {
	// Row contents longer than the screen width
	numberedLine := reader.NewFromTextForTesting("TestSearchHighlightTruncated", "1234abcdef").GetLine(linemetadata.Index{})
	pager := Pager{
		screen: twin.NewFakeScreen(6, 10),
		search: search.For("abcde"),
	}

	rendered := pager.renderLine(*numberedLine, pager.getLineNumberPrefixLength(numberedLine.Number), true)

	assert.Equal(t, len(rendered), 1) // No wrapping
	row := rendered[0]

	assert.Equal(t, len(row.cells), 6) // 1234a> (last being the scroll-right marker)

	expected := []textstyles.CellWithMetadata{
		{Rune: '1'},
		{Rune: '2'},
		{Rune: '3'},
		{Rune: '4'},
		{Rune: 'a', IsSearchHit: true, StartsSearchHit: true},
		{Rune: pager.ScrollRightHint.Rune},
	}

	for i, actualCell := range row.cells {
		expectedCell := expected[i]
		if actualCell.Rune != expectedCell.Rune || actualCell.IsSearchHit != expectedCell.IsSearchHit || actualCell.StartsSearchHit != expectedCell.StartsSearchHit {
			t.Fatalf("Cell %d mismatch, got\n%#v, want\n%#v", i, actualCell, expectedCell)
		}
	}
}

func TestOverflowDown(t *testing.T) {
	pager := Pager{
		screen: twin.NewFakeScreen(
			10, // Longer than the raw line, we're testing vertical overflow, not horizontal
			2,  // Single line of contents + one status line
		),

		// Single line of input
		readers: []*reader.ReaderImpl{reader.NewFromTextForTesting("test", "hej")},

		// This value can be anything and should be clipped, that's what we're testing
		scrollPosition: *scrollPositionFromIndex("TestOverflowDown", linemetadata.IndexFromOneBased(42)),
	}
	pager.filteringReader = FilteringReader{
		BackingReader: pager.readers[pager.currentReader],
		Filter:        &pager.filter,
	}

	rendered := pager.renderLines()
	assert.Equal(t, len(rendered.lines), 1)
	assert.Equal(t, "hej", renderedToString(rendered.lines[0].cells))
	assert.Equal(t, rendered.filenameText, "test")
	assert.Equal(t, rendered.statusText, ": 1 line  100%")
	assert.Assert(t, pager.lineIndex().IsZero())
	assert.Equal(t, pager.deltaScreenLines(), linemetadata.ScreenLines(0))
}

func TestOverflowUp(t *testing.T) {
	pager := Pager{
		screen: twin.NewFakeScreen(
			10, // Longer than the raw line, we're testing vertical overflow, not horizontal
			2,  // Single line of contents + one status line
		),

		// Single line of input
		readers: []*reader.ReaderImpl{reader.NewFromTextForTesting("test", "hej")},

		// NOTE: scrollPosition intentionally not initialized
	}
	pager.filteringReader = FilteringReader{
		BackingReader: pager.readers[pager.currentReader],
		Filter:        &pager.filter,
	}

	rendered := pager.renderLines()
	assert.Equal(t, len(rendered.lines), 1)
	assert.Equal(t, "hej", renderedToString(rendered.lines[0].cells))
	assert.Equal(t, rendered.filenameText, "test")
	assert.Equal(t, rendered.statusText, ": 1 line  100%")
	assert.Assert(t, pager.lineIndex().IsZero())
	assert.Equal(t, pager.deltaScreenLines(), linemetadata.ScreenLines(0))
}

func TestWrapping(t *testing.T) {
	reader := reader.NewFromTextForTesting("",
		"first line\nline two will be wrapped\nhere's the last line")
	pager := NewPager(reader)
	pager.screen = twin.NewFakeScreen(40, 40)

	pager.WrapLongLines = true
	pager.ShowLineNumbers = false
	pager.showLineNumbers = false

	assert.NilError(t, reader.Wait())

	// This is what we're testing really
	pager.scrollToEnd()

	// Higher than needed, we'll just be validating the necessary lines at the
	// top.
	screen := twin.NewFakeScreen(10, 99)

	// Exit immediately
	pager.Quit()

	// Get contents onto our fake screen
	pager.StartPaging(screen, nil, nil)
	pager.redraw("")

	actual := strings.Join([]string{
		rowToString(screen.GetRow(0)),
		rowToString(screen.GetRow(1)),
		rowToString(screen.GetRow(2)),
		rowToString(screen.GetRow(3)),
		rowToString(screen.GetRow(4)),
		rowToString(screen.GetRow(5)),
		rowToString(screen.GetRow(6)),
		rowToString(screen.GetRow(7)),
	}, "\n")
	assert.Equal(t, actual, strings.Join([]string{
		"first line",
		"line two",
		"will be",
		"wrapped",
		"here's the",
		"last line",
		"---",
		"",
	}, "\n"))
}

// Repro for https://github.com/walles/moor/issues/153
func TestOneLineTerminal(t *testing.T) {
	pager := Pager{
		// Single line terminal window, this is what we're testing
		screen: twin.NewFakeScreen(20, 1),

		readers:       []*reader.ReaderImpl{reader.NewFromTextForTesting("test", "hej")},
		ShowStatusBar: true,
	}
	pager.filteringReader = FilteringReader{
		BackingReader: pager.readers[pager.currentReader],
		Filter:        &pager.filter,
	}

	rendered := pager.renderLines()
	assert.Equal(t, len(rendered.lines), 0)
}

// What happens if we are scrolled to the bottom of a 1000 lines file, and then
// add a filter matching only the first line?
//
// What should happen is that we should go as far down as possible.
func TestShortenedInput(t *testing.T) {
	pager := Pager{
		screen: twin.NewFakeScreen(20, 10),

		// 1000 lines of input, we will scroll to the bottom
		readers: []*reader.ReaderImpl{reader.NewFromTextForTesting("test", "first\n"+strings.Repeat("line\n", 1000))},

		scrollPosition: newScrollPosition("TestShortenedInput"),
	}

	// Hide the status bar for this test
	pager.mode = PagerModeViewing{&pager}
	pager.ShowStatusBar = false

	pager.filteringReader = FilteringReader{
		BackingReader: pager.readers[pager.currentReader],
		Filter:        &pager.filter,
	}

	pager.scrollToEnd()
	assert.Equal(t, pager.lineIndex().Index(), 991, "This should have been the effect of calling scrollToEnd()")

	pager.mode = NewPagerModeFilter(&pager)
	pager.filter = search.For("first") // Match only the first line

	rendered := pager.renderLines()
	assert.Equal(t, len(rendered.lines), 1, "Should have rendered one line")
	assert.Equal(t, "first", renderedToString(rendered.lines[0].cells))
	assert.Equal(t, pager.lineIndex().Index(), 0, "Should have scrolled to the first line")
}

// - Start with a 1000 lines file
// - Scroll to the bottom
// - Add a filter matching the first 100 lines
// - Render
// - Verify that the 10 last matching lines were rendered
func TestShortenedInputManyLines(t *testing.T) {
	lines := []string{"first"}
	for i := range 999 {
		if i < 100 {
			lines = append(lines, "match "+strconv.Itoa(i))
		} else {
			lines = append(lines, "other "+strconv.Itoa(i))
		}
	}

	pager := Pager{
		screen:         twin.NewFakeScreen(20, 10),
		readers:        []*reader.ReaderImpl{reader.NewFromTextForTesting("test", strings.Join(lines, "\n"))},
		scrollPosition: newScrollPosition("TestShortenedInputManyLines"),
	}

	pager.filteringReader = FilteringReader{
		BackingReader: pager.readers[pager.currentReader],
		Filter:        &pager.filter,
	}

	pager.scrollToEnd()
	assert.Equal(t, pager.lineIndex().Index(), 991, "Should be at the last line before filtering")

	pager.mode = NewPagerModeFilter(&pager)
	pager.filter = search.For(`^match`)

	rendered := pager.renderLines()
	assert.Equal(t, len(rendered.lines), 9, "Should have rendered 9 lines (10 minus one status bar)")

	expectedLines := []string{}
	for i := 91; i < 100; i++ {
		expectedLines = append(expectedLines, "match "+strconv.Itoa(i))
	}
	for i, row := range rendered.lines {
		assert.Equal(t, renderedToString(row.cells), expectedLines[i], "Line %d mismatch", i)
	}
	assert.Equal(t, pager.lineIndex().Index(), 91, "The last lines should now be visible")
	assert.Equal(t, "match 99", renderedToString(rendered.lines[len(rendered.lines)-1].cells))
}

// Text lines are fewer than the number of screen lines. All lines have search
// hits.
//
// Expected: No search hit line highlighting, since highlighting all lines is
// sort of like highlighting no lines but with an uglier background.
func TestRenderLines_FewLinesAllWithSearchHits(t *testing.T) {
	testRenderLinesWithSearchHits(t,
		"1xxx",
		[]twin.Color{twin.ColorDefault},
		"TestRenderLines_FewLinesAllWithSearchHits",
	)
}

// Text lines are more than the number of screen lines. All lines on screen have
// search hits.
//
// Expected: No search hit line highlighting, since highlighting all lines is
// sort of like highlighting no lines but with an uglier background.
func TestRenderLines_ManyLinesAllWithSearchHits(t *testing.T) {
	testRenderLinesWithSearchHits(t,
		"1xxx\n2xxx\n3xxx\n4xxx",
		[]twin.Color{twin.ColorDefault, twin.ColorDefault, twin.ColorDefault},
		"TestRenderLines_ManyLinesAllWithSearchHits",
	)
}

// Some lines with search hits and one line without. The ones with search hits
// should be line highlighted.
func TestRenderLines_OneLineWithoutSearchHit(t *testing.T) {
	testRenderLinesWithSearchHits(t,
		"1xxx\n2miss\n3xxx\n4xxx",
		[]twin.Color{red, twin.ColorDefault, red},
		"TestRenderLines_OneLineWithoutSearchHit",
	)
}

var red = twin.NewColor24Bit(0xff, 0x00, 0x00)

// Helper for search hit line highlight tests.
//
// Search pattern is always "xxx", because "x" marks the spot. The
// expectedBackgrounds are the expected background colors for the first (always
// unmatched) letter on each line.
//
// The screen being rendered to is always 10 columns wide and 3 rows high.
func testRenderLinesWithSearchHits(t *testing.T, input string, expectedBackgrounds []twin.Color, scrollPositionName string) {
	r := reader.NewFromTextForTesting("test", input)
	pager := Pager{
		screen:         twin.NewFakeScreen(10, 3),
		readers:        []*reader.ReaderImpl{r},
		scrollPosition: newScrollPosition(scrollPositionName),
	}
	pager.filteringReader = FilteringReader{
		BackingReader: pager.readers[pager.currentReader],
		Filter:        &pager.filter,
	}
	pager.search = search.For("xxx")
	pager.ShowStatusBar = false
	pager.mode = PagerModeViewing{&pager}
	pager.showLineNumbers = false
	assert.NilError(t, r.Wait())

	searchHitLineBackground = &red

	rendered := pager.renderLines()
	assert.Equal(t, len(rendered.lines), len(expectedBackgrounds))
	for i := range expectedBackgrounds {
		assert.Equal(t, rendered.lines[i].cells[0].Style.Background(), expectedBackgrounds[i])
	}
}

func BenchmarkRenderLines(b *testing.B) {
	input := reader.NewFromTextForTesting(
		"BenchmarkRenderLine()",
		strings.Repeat("This is a line with text and some more text to make it long enough.\n", 100))
	pager := NewPager(input)
	pager.screen = twin.NewFakeScreen(80, 25)

	assert.NilError(b, input.Wait())

	pager.renderLines() // Warm up

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pager.renderLines()
	}
}

// Inspired by https://github.com/walles/moor/issues/358
func BenchmarkRenderHugeLine(b *testing.B) {
	log.SetLevel(log.WarnLevel) // Stop info logs from polluting benchmark output

	const megabytes = 5
	builder := strings.Builder{}
	for builder.Len() < megabytes*1024*1024 {
		builder.WriteString("Romani ite domum. ")
	}
	b.SetBytes(int64(builder.Len()))

	input := reader.NewFromTextForTesting(
		"BenchmarkRenderHugeLine()",
		builder.String())
	pager := NewPager(input)
	pager.screen = twin.NewFakeScreen(80, 25)

	assert.NilError(b, input.Wait())

	pager.renderLines() // Warm up

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pager.renderLines()
	}
}

// See https://github.com/walles/moor/issues/412
func BenchmarkRenderHugeLineWithSearch(b *testing.B) {
	log.SetLevel(log.WarnLevel) // Stop info logs from polluting benchmark output

	const megabytes = 5
	builder := strings.Builder{}
	for builder.Len() < megabytes*1024*1024 {
		builder.WriteString("Romani ite domum. ")
	}
	b.SetBytes(int64(builder.Len()))

	input := reader.NewFromTextForTesting(
		"BenchmarkRenderHugeLineWithSearch()",
		builder.String())
	pager := NewPager(input)
	pager.screen = twin.NewFakeScreen(80, 25)
	pager.search = search.For("domum")

	assert.NilError(b, input.Wait())

	pager.renderLines() // Warm up

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pager.renderLines()
	}
}
