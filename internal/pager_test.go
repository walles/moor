package internal

import (
	"fmt"
	"os"
	"path"
	"runtime"
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/google/go-cmp/cmp"
	"github.com/walles/moor/v2/internal/linemetadata"
	"github.com/walles/moor/v2/internal/reader"
	"github.com/walles/moor/v2/internal/textstyles"
	"github.com/walles/moor/v2/twin"
	"gotest.tools/v3/assert"
)

// NOTE: You can find related tests in screenLines_test.go.

const blueBackgroundClearToEol0 = "\x1b[44m\x1b[0K" // With 0 before the K, should clear to EOL
const blueBackgroundClearToEol = "\x1b[44m\x1b[K"   // No 0 before the K, should also clear to EOL

const samplesDir = "../sample-files"

func TestUnicodeRendering(t *testing.T) {
	reader := reader.NewFromTextForTesting("", "åäö")

	var answers = []twin.StyledRune{
		twin.NewStyledRune('å', twin.StyleDefault),
		twin.NewStyledRune('ä', twin.StyleDefault),
		twin.NewStyledRune('ö', twin.StyleDefault),
	}

	contents := startPaging(t, reader).GetRow(0)
	for pos, expected := range answers {
		assertRunesEqual(t, expected, contents[pos])
	}
}

func assertRunesEqual(t *testing.T, expected twin.StyledRune, actual twin.StyledRune) {
	t.Helper()

	if actual.Rune == expected.Rune && actual.Style == expected.Style {
		return
	}

	t.Errorf("Expected %v, got %v", expected, actual)
}

func TestFgColorRendering(t *testing.T) {
	reader := reader.NewFromTextForTesting("",
		"\x1b[30ma\x1b[31mb\x1b[32mc\x1b[33md\x1b[34me\x1b[35mf\x1b[36mg\x1b[37mh\x1b[0mi")

	var answers = []twin.StyledRune{
		twin.NewStyledRune('a', twin.StyleDefault.WithForeground(twin.NewColor16(0))),
		twin.NewStyledRune('b', twin.StyleDefault.WithForeground(twin.NewColor16(1))),
		twin.NewStyledRune('c', twin.StyleDefault.WithForeground(twin.NewColor16(2))),
		twin.NewStyledRune('d', twin.StyleDefault.WithForeground(twin.NewColor16(3))),
		twin.NewStyledRune('e', twin.StyleDefault.WithForeground(twin.NewColor16(4))),
		twin.NewStyledRune('f', twin.StyleDefault.WithForeground(twin.NewColor16(5))),
		twin.NewStyledRune('g', twin.StyleDefault.WithForeground(twin.NewColor16(6))),
		twin.NewStyledRune('h', twin.StyleDefault.WithForeground(twin.NewColor16(7))),
		twin.NewStyledRune('i', twin.StyleDefault),
	}

	contents := startPaging(t, reader).GetRow(0)
	for pos, expected := range answers {
		assertRunesEqual(t, expected, contents[pos])
	}
}

func TestPageEmpty(t *testing.T) {
	// "---" is the eofSpinner of pager.go
	assert.Equal(t, "---", renderTextLine(""))
}

func TestBrokenUtf8(t *testing.T) {
	// The broken UTF8 character in the middle is based on "©" = 0xc2a9
	reader := reader.NewFromTextForTesting("", "abc\xc2def")

	var answers = []twin.StyledRune{
		twin.NewStyledRune('a', twin.StyleDefault),
		twin.NewStyledRune('b', twin.StyleDefault),
		twin.NewStyledRune('c', twin.StyleDefault),
		twin.NewStyledRune('?', twin.StyleDefault.WithForeground(twin.NewColor16(7)).WithBackground(twin.NewColor16(1))),
		twin.NewStyledRune('d', twin.StyleDefault),
		twin.NewStyledRune('e', twin.StyleDefault),
		twin.NewStyledRune('f', twin.StyleDefault),
	}

	contents := startPaging(t, reader).GetRow(0)
	for pos, expected := range answers {
		assertRunesEqual(t, expected, contents[pos])
	}
}

func startPaging(t *testing.T, reader *reader.ReaderImpl) *twin.FakeScreen {
	// 0 means default tab size. Defaults to 8 to be like less.
	return startPagingWithTabSizeAndScreen(t, 0, twin.NewFakeScreen(20, 10), reader)
}

func startPagingWithTabSize(t *testing.T, tabSize int, reader *reader.ReaderImpl) *twin.FakeScreen {
	return startPagingWithTabSizeAndScreen(t, tabSize, twin.NewFakeScreen(20, 10), reader)
}

func startPagingWithScreen(t *testing.T, screen *twin.FakeScreen, reader *reader.ReaderImpl) *twin.FakeScreen {
	// 0 means default tab size. Defaults to 8 to be like less.
	return startPagingWithTabSizeAndScreen(t, 0, screen, reader)
}

func startPagingWithTabSizeAndScreen(t *testing.T, tabSize int, screen *twin.FakeScreen, reader *reader.ReaderImpl) *twin.FakeScreen {
	err := reader.Wait()
	if err != nil {
		t.Fatalf("Failed waiting for reader: %v", err)
	}

	pager := NewPager(reader)
	pager.TabSize = tabSize
	pager.ShowLineNumbers = false
	pager.showLineNumbers = false

	// Tell our Pager to quit immediately
	pager.Quit()

	// Except for just quitting, this also associates our FakeScreen with the Pager
	pager.StartPaging(screen, nil, nil)

	// This makes sure at least one frame gets rendered
	pager.redraw("")

	return screen
}

// Set style to "native" and use the TTY16m formatter
func startPagingWithTerminalFg(t *testing.T, reader *reader.ReaderImpl, withTerminalFg bool) *twin.FakeScreen {
	err := reader.Wait()
	if err != nil {
		t.Fatalf("Failed waiting for reader: %v", err)
	}

	screen := twin.NewFakeScreen(20, 10)
	pager := NewPager(reader)
	pager.ShowLineNumbers = false
	pager.showLineNumbers = false
	pager.WithTerminalFg = withTerminalFg

	// Tell our Pager to quit immediately
	pager.Quit()

	// Except for just quitting, this also associates our FakeScreen with the Pager
	pager.StartPaging(screen, styles.Get("native"), &formatters.TTY16m)

	// This makes sure at least one frame gets rendered
	pager.redraw("")

	return screen
}

// assertIndexOfFirstX verifies the (zero-based) index of the first 'x'
func assertIndexOfFirstX(t *testing.T, tabSize int, s string, expectedIndex int) {
	reader := reader.NewFromTextForTesting("", s)

	contents := startPagingWithTabSize(t, tabSize, reader).GetRow(0)
	for pos, cell := range contents {
		if cell.Rune != 'x' {
			continue
		}

		if pos == expectedIndex {
			// Success!
			return
		}

		t.Errorf("Expected first 'x' with tab size %d to be at (zero-based) index %d, but was at %d: \"%s\"",
			tabSize, expectedIndex, pos, strings.ReplaceAll(s, "\x09", "<TAB>"))
		return
	}

	panic("No 'x' found")
}

func TestTabHandling(t *testing.T) {
	assertIndexOfFirstX(t, 4, "x", 0)

	assertIndexOfFirstX(t, 4, "\x09x", 4)
	assertIndexOfFirstX(t, 4, "\x09\x09x", 8)

	assertIndexOfFirstX(t, 4, "J\x09x", 4)
	assertIndexOfFirstX(t, 4, "Jo\x09x", 4)
	assertIndexOfFirstX(t, 4, "Joh\x09x", 4)
	assertIndexOfFirstX(t, 4, "Joha\x09x", 8)
	assertIndexOfFirstX(t, 4, "Johan\x09x", 8)

	assertIndexOfFirstX(t, 4, "\x09J\x09x", 8)
	assertIndexOfFirstX(t, 4, "\x09Jo\x09x", 8)
	assertIndexOfFirstX(t, 4, "\x09Joh\x09x", 8)
	assertIndexOfFirstX(t, 4, "\x09Joha\x09x", 12)
	assertIndexOfFirstX(t, 4, "\x09Johan\x09x", 12)
}

func TestTabHandling_TabSize8(t *testing.T) {
	assertIndexOfFirstX(t, 8, "\x09x", 8)
}

func TestCodeHighlighting(t *testing.T) {
	// From: https://coderwall.com/p/_fmbug/go-get-path-to-current-file
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		panic("Getting current filename failed")
	}

	reader, err := reader.NewFromFilename(filename, formatters.TTY16m, reader.ReaderOptions{Style: styles.Get("native")})
	assert.NilError(t, err)
	assert.NilError(t, reader.Wait())

	packageKeywordStyle := twin.StyleDefault.WithAttr(twin.AttrBold).WithForeground(twin.NewColorHex(0x6AB825))
	packageSpaceStyle := twin.StyleDefault.WithForeground(twin.NewColorHex(0x666666))
	packageNameStyle := twin.StyleDefault.WithForeground(twin.NewColorHex(0xD0D0D0))
	var answers = []twin.StyledRune{
		twin.NewStyledRune('p', packageKeywordStyle),
		twin.NewStyledRune('a', packageKeywordStyle),
		twin.NewStyledRune('c', packageKeywordStyle),
		twin.NewStyledRune('k', packageKeywordStyle),
		twin.NewStyledRune('a', packageKeywordStyle),
		twin.NewStyledRune('g', packageKeywordStyle),
		twin.NewStyledRune('e', packageKeywordStyle),
		twin.NewStyledRune(' ', packageSpaceStyle),
		twin.NewStyledRune('i', packageNameStyle),
		twin.NewStyledRune('n', packageNameStyle),
		twin.NewStyledRune('t', packageNameStyle),
		twin.NewStyledRune('e', packageNameStyle),
		twin.NewStyledRune('r', packageNameStyle),
		twin.NewStyledRune('n', packageNameStyle),
		twin.NewStyledRune('a', packageNameStyle),
		twin.NewStyledRune('l', packageNameStyle),
	}

	contents := startPaging(t, reader).GetRow(0)
	for pos, expected := range answers {
		assertRunesEqual(t, expected, contents[pos])
	}
}

func TestCodeHighlight_compressed(t *testing.T) {
	// Same as TestCodeHighlighting but with "compressed-markdown.md.gz"
	reader, err := reader.NewFromFilename("../sample-files/compressed-markdown.md.gz", formatters.TTY16m, reader.ReaderOptions{Style: styles.Get("native")})
	assert.NilError(t, err)
	assert.NilError(t, reader.Wait())

	markdownHeading1Style := twin.StyleDefault.WithAttr(twin.AttrBold).WithForeground(twin.NewColorHex(0xffffff))
	var answers = []twin.StyledRune{
		twin.NewStyledRune('#', markdownHeading1Style),
		twin.NewStyledRune(' ', markdownHeading1Style),
		twin.NewStyledRune('M', markdownHeading1Style),
		twin.NewStyledRune('a', markdownHeading1Style),
		twin.NewStyledRune('r', markdownHeading1Style),
		twin.NewStyledRune('k', markdownHeading1Style),
		twin.NewStyledRune('d', markdownHeading1Style),
		twin.NewStyledRune('o', markdownHeading1Style),
		twin.NewStyledRune('w', markdownHeading1Style),
		twin.NewStyledRune('n', markdownHeading1Style),
	}

	contents := startPaging(t, reader).GetRow(0)
	for pos, expected := range answers {
		assertRunesEqual(t, expected, contents[pos])
	}
}

// Regression test for:
// https://github.com/walles/moor/issues/236#issuecomment-2282677792
//
// Sample file sysctl.h from:
// https://github.com/fastfetch-cli/fastfetch/blob/f9597eba39d6afd278eeca2f2972f73a7e54f111/src/common/sysctl.h
func TestCodeHighlightingIncludes(t *testing.T) {
	reader, err := reader.NewFromFilename("../sample-files/sysctl.h", formatters.TTY16m, reader.ReaderOptions{Style: styles.Get("native")})
	assert.NilError(t, err)
	assert.NilError(t, reader.Wait())

	screen := startPaging(t, reader)
	firstIncludeLine := screen.GetRow(2)
	secondIncludeLine := screen.GetRow(3)

	// Both should start with "#include" colored the same way
	assertRunesEqual(t, firstIncludeLine[0], secondIncludeLine[0])
}

func TestUnicodePrivateUse(t *testing.T) {
	// This character lives in a Private Use Area:
	// https://codepoints.net/U+f244
	//
	// It's used by Font Awesome as "fa-battery-empty":
	// https://fontawesome.com/v4/icon/battery-empty
	char := '\uf244'

	reader := reader.NewFromTextForTesting("hello", string(char))
	renderedRune := startPaging(t, reader).GetRow(0)[0]

	// Make sure we display this character unmodified
	assertRunesEqual(t, twin.NewStyledRune(char, twin.StyleDefault), renderedRune)
}

func resetManPageFormat() {
	textstyles.ManPageBold = twin.StyleDefault.WithAttr(twin.AttrBold)
	textstyles.ManPageUnderline = twin.StyleDefault.WithAttr(twin.AttrUnderline)
}

func testManPageFormatting(t *testing.T, input string, expected twin.StyledRune) {
	t.Helper()

	reader := reader.NewFromTextForTesting("", input)

	// Without these lines the man page tests will fail if either of these
	// environment variables are set when the tests are run.
	assert.NilError(t, os.Setenv("LESS_TERMCAP_md", ""))
	assert.NilError(t, os.Setenv("LESS_TERMCAP_us", ""))
	assert.NilError(t, os.Setenv("LESS_TERMCAP_so", ""))
	resetManPageFormat()

	contents := startPaging(t, reader).GetRow(0)
	assertRunesEqual(t, expected, contents[0])
	assert.Equal(t, contents[1].Rune, ' ')
}

func TestManPageFormatting(t *testing.T) {
	testManPageFormatting(t, "n\x08n", twin.NewStyledRune('n', twin.StyleDefault.WithAttr(twin.AttrBold)))
	testManPageFormatting(t, "_\x08x", twin.NewStyledRune('x', twin.StyleDefault.WithAttr(twin.AttrUnderline)))

	// Non-breaking space UTF-8 encoded (0xc2a0) should render as a non-breaking unicode space (0xa0)
	testManPageFormatting(t, string([]byte{0xc2, 0xa0}), twin.NewStyledRune(rune(0xa0), twin.StyleDefault))

	// Corner cases
	testManPageFormatting(t, "\x08", twin.NewStyledRune('<', twin.StyleDefault.WithForeground(twin.NewColor16(7)).WithBackground(twin.NewColor16(1))))

	// FIXME: Test two consecutive backspaces

	// FIXME: Test backspace between two uncombinable characters
}

func TestScrollToBottomWrapNextToLastLine(t *testing.T) {
	reader := reader.NewFromTextForTesting("",
		"first line\nline two will be wrapped\nhere's the last line")

	// Heigh 3 = two lines of contents + one footer
	screen := twin.NewFakeScreen(10, 3)

	pager := NewPager(reader)
	pager.WrapLongLines = true
	pager.ShowLineNumbers = false
	pager.showLineNumbers = false
	pager.screen = screen

	assert.NilError(t, pager.readers[pager.currentReader].Wait())

	// This is what we're testing really
	pager.scrollToEnd()

	// Exit immediately
	pager.Quit()

	// Get contents onto our fake screen
	pager.StartPaging(screen, nil, nil)
	pager.redraw("")

	actual := strings.Join([]string{
		rowToString(screen.GetRow(0)),
		rowToString(screen.GetRow(1)),
		rowToString(screen.GetRow(2)),
	}, "\n")
	expected := strings.Join([]string{
		"here's the",
		"last line",
		"3 lines  1", // "3 lines 100%" clipped after 10 characters (screen width)
	}, "\n")
	assert.Equal(t, actual, expected)
}

// Repro for https://github.com/walles/moor/issues/105
func TestScrollToEndLongInput(t *testing.T) {
	const lineCount = 10100 // At least five digits

	// "X" marks the spot
	reader := reader.NewFromTextForTesting("test", strings.Repeat(".\n", lineCount-1)+"X")
	pager := NewPager(reader)
	pager.ShowLineNumbers = true
	pager.showLineNumbers = true

	// Tell our Pager to quit immediately
	pager.Quit()

	// Connect the pager with a screen
	const screenHeight = 10
	screen := twin.NewFakeScreen(20, screenHeight)
	pager.StartPaging(screen, nil, nil)

	// This is what we're really testing
	pager.scrollToEnd()

	// This makes sure at least one frame gets rendered
	pager.redraw("")

	// The last screen line holds the status field, and the next to last screen
	// line holds the last contents line.
	lastContentsLine := screen.GetRow(screenHeight - 2)
	firstContentsColumn := len("10_100 ")
	assertRunesEqual(t, twin.NewStyledRune('X', twin.StyleDefault), lastContentsLine[firstContentsColumn])
}

func TestIsScrolledToEnd_LongFile(t *testing.T) {
	// Six lines of contents
	reader := reader.NewFromTextForTesting("Testing", "a\nb\nc\nd\ne\nf\n")

	// Three lines screen
	screen := twin.NewFakeScreen(20, 3)

	// Create the pager
	pager := NewPager(reader)
	pager.screen = screen

	assert.Equal(t, false, pager.isScrolledToEnd())

	pager.scrollToEnd()
	assert.Equal(t, true, pager.isScrolledToEnd())
}

func TestIsScrolledToEnd_ShortFile(t *testing.T) {
	// Three lines of contents
	reader := reader.NewFromTextForTesting("Testing", "a\nb\nc")

	// Six lines screen
	screen := twin.NewFakeScreen(20, 6)

	// Create the pager
	pager := NewPager(reader)
	pager.screen = screen

	assert.Equal(t, true, pager.isScrolledToEnd())

	pager.scrollToEnd()
	assert.Equal(t, true, pager.isScrolledToEnd())
}

func TestIsScrolledToEnd_ExactFile(t *testing.T) {
	// Three lines of contents
	reader := reader.NewFromTextForTesting("Testing", "a\nb\nc")

	// Three lines screen
	screen := twin.NewFakeScreen(20, 3)

	// Create the pager
	pager := NewPager(reader)
	pager.screen = screen
	pager.ShowStatusBar = false

	assert.Equal(t, true, pager.isScrolledToEnd())

	pager.scrollToEnd()
	assert.Equal(t, true, pager.isScrolledToEnd())
}

func TestIsScrolledToEnd_WrappedLastLine(t *testing.T) {
	// Three lines of contents
	reader := reader.NewFromTextForTesting("Testing", "a\nb\nc d e f g h i j k l m n")

	// Three lines screen
	screen := twin.NewFakeScreen(5, 3)

	// Create the pager
	pager := NewPager(reader)
	pager.screen = screen
	pager.WrapLongLines = true

	assert.Equal(t, false, pager.isScrolledToEnd())

	pager.scrollToEnd()
	assert.Equal(t, true, pager.isScrolledToEnd())

	pager.mode.onKey(twin.KeyUp)
	pager.redraw("XXX")
	assert.Equal(t, false, pager.isScrolledToEnd())
}

func TestIsScrolledToEnd_EmptyFile(t *testing.T) {
	// No contents
	reader := reader.NewFromTextForTesting("Testing", "")

	// Three lines screen
	screen := twin.NewFakeScreen(20, 3)

	// Create the pager
	pager := NewPager(reader)
	pager.screen = screen

	assert.Equal(t, true, pager.isScrolledToEnd())

	pager.scrollToEnd()
	assert.Equal(t, true, pager.isScrolledToEnd())
}

func getTestFiles(t *testing.T) []string {
	files, err := os.ReadDir(samplesDir)
	assert.NilError(t, err)

	var filenames []string
	for _, file := range files {
		filenames = append(filenames, path.Join(samplesDir, file.Name()))
	}

	return filenames
}

// Verify that we can page all files in ../sample-files/* without crashing
func TestPageSamples(t *testing.T) {
	for _, fileName := range getTestFiles(t) {
		t.Run(fileName, func(t *testing.T) {
			file, err := os.Open(fileName)
			if err != nil {
				t.Errorf("Error opening file <%s>: %s", fileName, err.Error())
				return
			}

			myReader, err := reader.NewFromStream(fileName, file, nil, reader.ReaderOptions{Style: &chroma.Style{}})
			assert.NilError(t, err)
			assert.NilError(t, myReader.Wait())

			pager := NewPager(myReader)
			pager.WrapLongLines = false
			pager.ShowLineNumbers = false
			pager.showLineNumbers = false

			// Heigh 3 = two lines of contents + one footer
			screen := twin.NewFakeScreen(10, 3)

			// Exit immediately
			pager.Quit()

			// Get contents onto our fake screen
			pager.StartPaging(screen, nil, nil)
			pager.redraw("")

			firstReaderLine := myReader.GetLine(linemetadata.Index{})
			if firstReaderLine == nil {
				return
			}
			firstPagerLine := rowToString(screen.GetRow(0))

			// Handle the case when first line is chopped off to the right
			firstPagerLine = strings.TrimSuffix(firstPagerLine, ">")

			assert.Assert(t,
				strings.HasPrefix(firstReaderLine.Plain(), firstPagerLine),
				"\nreader line = <%s>\npager line  = <%s>",
				firstReaderLine.Plain(), firstPagerLine,
			)
		})
	}
}

// Validate rendering of https://en.wikipedia.org/wiki/ANSI_escape_code#EL
func TestClearToEndOfLine_ClearFromStart(t *testing.T) {
	screen := startPaging(t, reader.NewFromTextForTesting("TestClearToEol", blueBackgroundClearToEol))

	screenWidth, _ := screen.Size()
	var expected []twin.StyledRune
	for len(expected) < screenWidth {
		expected = append(expected,
			twin.NewStyledRune(' ', twin.StyleDefault.WithBackground(twin.NewColor16(4))),
		)
	}

	actual := screen.GetRow(0)
	assert.DeepEqual(t, actual, expected, cmp.AllowUnexported(twin.Style{}))
}

// Validate rendering of https://en.wikipedia.org/wiki/ANSI_escape_code#EL
func TestClearToEndOfLine_ClearFromNotStart(t *testing.T) {
	screen := startPaging(t, reader.NewFromTextForTesting("TestClearToEol", "a"+blueBackgroundClearToEol))

	screenWidth, _ := screen.Size()
	expected := []twin.StyledRune{
		twin.NewStyledRune('a', twin.StyleDefault),
	}
	for len(expected) < screenWidth {
		expected = append(expected,
			twin.NewStyledRune(' ', twin.StyleDefault.WithBackground(twin.NewColor16(4))),
		)
	}

	actual := screen.GetRow(0)
	assert.DeepEqual(t, actual, expected, cmp.AllowUnexported(twin.Style{}))
}

// Validate rendering of https://en.wikipedia.org/wiki/ANSI_escape_code#EL
func TestClearToEndOfLine_ClearFromStartScrolledRight(t *testing.T) {
	pager := NewPager(reader.NewFromTextForTesting("TestClearToEol", blueBackgroundClearToEol0))
	pager.ShowLineNumbers = false
	pager.showLineNumbers = false

	// Tell our Pager to quit immediately
	pager.Quit()

	// Except for just quitting, this also associates a FakeScreen with the Pager
	screen := twin.NewFakeScreen(3, 10)
	pager.StartPaging(screen, nil, nil)

	// Scroll right, this is what we're testing
	pager.leftColumnZeroBased = 44

	// This makes sure at least one frame gets rendered
	pager.redraw("")

	screenWidth, _ := screen.Size()
	var expected []twin.StyledRune
	for len(expected) < screenWidth {
		expected = append(expected,
			twin.NewStyledRune(' ', twin.StyleDefault.WithBackground(twin.NewColor16(4))),
		)
	}

	actual := screen.GetRow(0)
	assert.DeepEqual(t, actual, expected, cmp.AllowUnexported(twin.Style{}))
}

// Render a line of text on our 20 cell wide screen
func renderTextLine(text string) string {
	reader := reader.NewFromTextForTesting("renderTextLine", text)
	screen := startPaging(nil, reader)
	return rowToString(screen.GetRow(0))
}

// Ref: https://github.com/walles/moor/issues/243
func TestPageWideChars(t *testing.T) {
	// Both of these characters are 2 cells wide on a terminal
	const monospaced4cells = "上午"
	const monospaced8cells = monospaced4cells + monospaced4cells
	const monospaced16cells = monospaced8cells + monospaced8cells
	const monospaced20cells = monospaced16cells + monospaced4cells
	const monospaced24cells = monospaced16cells + monospaced8cells

	// Cut the line in the middle of a wide character
	const monospaced18cells = monospaced16cells + "上"
	assert.Equal(t, monospaced18cells+" >", renderTextLine(monospaced24cells))

	// Just the right length, no cutting
	assert.Equal(t, monospaced20cells, renderTextLine(monospaced20cells))

	// Cut this line after a whide character
	assert.Equal(t, "x"+monospaced18cells+">", renderTextLine("x"+monospaced24cells))
}

func TestTerminalFg(t *testing.T) {
	reader := reader.NewFromTextForTesting("", "x")

	var styleAnswer = twin.NewStyledRune('x', twin.StyleDefault.WithForeground(twin.NewColor24Bit(0xd0, 0xd0, 0xd0)))
	var terminalAnswer = twin.NewStyledRune('x', twin.StyleDefault)

	assertRunesEqual(t, styleAnswer, startPagingWithTerminalFg(t, reader, false).GetRow(0)[0])
	assertRunesEqual(t, terminalAnswer, startPagingWithTerminalFg(t, reader, true).GetRow(0)[0])
}

func testFooter(t *testing.T, filename string, contents string, expectedFooter string) {
	reader := reader.NewFromTextForTesting(filename, contents)
	screen := startPagingWithScreen(t, twin.NewFakeScreen(999, 10), reader)
	footer := rowToString(screen.GetRow(9))
	assert.Equal(t, expectedFooter, footer, fmt.Sprintf("filename='%s', contents='%s'", filename, contents))
}

func TestFooter(t *testing.T) {
	help := "Press escape to exit, / to search, & to filter, h for help"

	testFooter(t, "filename", "", "filename: <empty>  "+help)
	testFooter(t, "", "", "<empty>  "+help)
	testFooter(t, "", "text", "1 line  100%  "+help)
	testFooter(t, "filename", "text", "filename: 1 line  100%  "+help)

	testFooter(t, "", "line 1\nline 2", "2 lines  100%  "+help)
}

// Regression test for crash when following an empty file.
// Before the fix, IndexFromLength(0) would return nil, and calling .IsBefore()
// on nil would crash.
func TestHandleMoreLinesAvailableWithEmptyFile(t *testing.T) {
	// Create a pager with an empty reader
	emptyReader := reader.NewFromTextForTesting("empty", "")
	pager := NewPager(emptyReader)

	// Simulate --follow mode by setting target to max
	targetLine := linemetadata.IndexMax()
	pager.TargetLine = &targetLine

	// This should not crash when lineCount is 0
	pager.handleMoreLinesAvailable()

	// Verify target line is still set (we're still waiting for lines)
	if pager.TargetLine == nil {
		t.Error("Expected TargetLine to remain set when no lines available")
	}
}
