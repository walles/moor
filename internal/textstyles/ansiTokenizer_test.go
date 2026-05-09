package textstyles

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/google/go-cmp/cmp"
	log "github.com/sirupsen/logrus"

	"github.com/walles/moor/v2/internal/linemetadata"
	"github.com/walles/moor/v2/twin"
	"gotest.tools/v3/assert"
)

const samplesDir = "../../sample-files"

// Convert a cells array to a plain string
func cellsToPlainString(cells []CellWithMetadata) string {
	returnMe := ""
	for _, cell := range cells {
		returnMe += string(cell.Rune)
	}

	return returnMe
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

// Verify that we can tokenize all lines in ../sample-files/*
// without logging any errors
func TestTokenize(t *testing.T) {
	log.SetLevel(log.DebugLevel)

	for _, fileName := range getTestFiles(t) {
		t.Run(fileName, func(t *testing.T) {
			file, err := os.Open(fileName)
			if err != nil {
				t.Errorf("Error opening file <%s>: %s", fileName, err.Error())
				return
			}
			defer func() {
				if err := file.Close(); err != nil {
					panic(err)
				}
			}()

			fileReader, err := os.Open(fileName)
			assert.NilError(t, err)

			fileScanner := bufio.NewScanner(fileReader)

			// Upping the buffer like this (from a default of 64kb) makes the
			// tests go faster
			fileScanner.Buffer(make([]byte, 1024*1024), 1024*1024)

			var lineIndex *linemetadata.Index
			for fileScanner.Scan() {
				line := fileScanner.Text()
				if lineIndex == nil {
					lineIndex = &linemetadata.Index{}
				} else {
					next := lineIndex.NonWrappingAdd(1)
					lineIndex = &next
				}

				var loglines strings.Builder
				log.SetOutput(&loglines)

				tokens := StyledRunesFromString(twin.StyleDefault, line, lineIndex, 0).StyledRunes
				plainString := StripFormatting(line, *lineIndex, 0)
				if len(tokens) != utf8.RuneCountInString(plainString) {
					t.Errorf("%s:%s: len(tokens)=%d, len(plainString)=%d for: <%s>",
						fileName, lineIndex.Format(),
						len(tokens), utf8.RuneCountInString(plainString), line)
					continue
				}

				// Tokens and plain have the same lengths, compare contents
				plainStringChars := []rune(plainString)
				for index, plainChar := range plainStringChars {
					cellChar := tokens[index]
					if cellChar.Rune == plainChar {
						continue
					}

					if cellChar.Rune == '•' && plainChar == 'o' {
						// Pretty bullets on man pages
						continue
					}

					// Chars mismatch!
					plainStringFromCells := cellsToPlainString(tokens)
					positionMarker := strings.Repeat(" ", index) + "^"
					cellCharString := string(cellChar.Rune)
					if !twin.Printable(cellChar.Rune) {
						cellCharString = fmt.Sprint(int(cellChar.Rune))
					}
					plainCharString := string(plainChar)
					if !twin.Printable(plainChar) {
						plainCharString = fmt.Sprint(int(plainChar))
					}
					t.Errorf("%s:%s, 0-based column %d: cell char <%s> != plain char <%s>:\nPlain: %s\nCells: %s\n       %s",
						fileName, lineIndex.Format(), index,
						cellCharString, plainCharString,
						plainString,
						plainStringFromCells,
						positionMarker,
					)
					break
				}

				if len(loglines.String()) != 0 {
					t.Errorf("%s: %s", fileName, loglines.String())
					continue
				}
			}
		})
	}
}

func TestAllSupportedTextAttributes(t *testing.T) {
	testCases := []struct {
		name    string
		onCode  string
		offCode string
		attr    twin.AttrMask
	}{
		{name: "bold", onCode: "1", offCode: "22", attr: twin.AttrBold},
		{name: "dim", onCode: "2", offCode: "22", attr: twin.AttrDim},
		{name: "italic", onCode: "3", offCode: "23", attr: twin.AttrItalic},
		{name: "underline", onCode: "4", offCode: "24", attr: twin.AttrUnderline},
		{name: "blink", onCode: "5", offCode: "25", attr: twin.AttrBlink},
		{name: "reverse", onCode: "7", offCode: "27", attr: twin.AttrReverse},
		{name: "hidden", onCode: "8", offCode: "28", attr: twin.AttrHidden},
		{name: "strikethrough", onCode: "9", offCode: "29", attr: twin.AttrStrikeThrough},
	}

	for _, testCase := range testCases {
		tc := testCase
		t.Run(tc.name, func(t *testing.T) {
			input := "a\x1b[" + tc.onCode + "mb\x1b[" + tc.offCode + "mc"
			tokens := StyledRunesFromString(twin.StyleDefault, input, nil, 0).StyledRunes

			assert.Equal(t, len(tokens), 3)
			assert.Equal(t, tokens[0], CellWithMetadata{Rune: 'a', Style: twin.StyleDefault})
			assert.Equal(t, tokens[1], CellWithMetadata{Rune: 'b', Style: twin.StyleDefault.WithAttr(tc.attr)})
			assert.Equal(t, tokens[2], CellWithMetadata{Rune: 'c', Style: twin.StyleDefault})
		})
	}
}

func TestManPages(t *testing.T) {
	// Bold
	tokens := StyledRunesFromString(twin.StyleDefault, "ab\bbc", nil, 0).StyledRunes
	assert.Equal(t, len(tokens), 3)
	assert.Equal(t, tokens[0], CellWithMetadata{Rune: 'a', Style: twin.StyleDefault})
	assert.Equal(t, tokens[1], CellWithMetadata{Rune: 'b', Style: twin.StyleDefault.WithAttr(twin.AttrBold)})
	assert.Equal(t, tokens[2], CellWithMetadata{Rune: 'c', Style: twin.StyleDefault})

	// Underline
	tokens = StyledRunesFromString(twin.StyleDefault, "a_\bbc", nil, 0).StyledRunes
	assert.Equal(t, len(tokens), 3)
	assert.Equal(t, tokens[0], CellWithMetadata{Rune: 'a', Style: twin.StyleDefault})
	assert.Equal(t, tokens[1], CellWithMetadata{Rune: 'b', Style: twin.StyleDefault.WithAttr(twin.AttrUnderline)})
	assert.Equal(t, tokens[2], CellWithMetadata{Rune: 'c', Style: twin.StyleDefault})

	// Bullet point 1, taken from doing this on my macOS system:
	// env PAGER="hexdump -C" man printf | moor
	tokens = StyledRunesFromString(twin.StyleDefault, "a+\b+\bo\bob", nil, 0).StyledRunes
	assert.Equal(t, len(tokens), 3)
	assert.Equal(t, tokens[0], CellWithMetadata{Rune: 'a', Style: twin.StyleDefault})
	assert.Equal(t, tokens[1], CellWithMetadata{Rune: '•', Style: twin.StyleDefault})
	assert.Equal(t, tokens[2], CellWithMetadata{Rune: 'b', Style: twin.StyleDefault})

	// Bullet point 2, taken from doing this using the "fish" shell on my macOS system:
	// man printf | hexdump -C | moor
	tokens = StyledRunesFromString(twin.StyleDefault, "a+\bob", nil, 0).StyledRunes
	assert.Equal(t, len(tokens), 3)
	assert.Equal(t, tokens[0], CellWithMetadata{Rune: 'a', Style: twin.StyleDefault})
	assert.Equal(t, tokens[1], CellWithMetadata{Rune: '•', Style: twin.StyleDefault})
	assert.Equal(t, tokens[2], CellWithMetadata{Rune: 'b', Style: twin.StyleDefault})
}

func TestManPageHeadings(t *testing.T) {
	// Set a marker style we can recognize and test for
	ManPageHeading = twin.StyleDefault.WithForeground(twin.NewColor16(2))

	manPageHeading := ""
	for _, char := range "JOHAN HELLO" {
		manPageHeading += string(char) + "\b" + string(char)
	}

	notAllCaps := ""
	for _, char := range "Johan Hello" {
		notAllCaps += string(char) + "\b" + string(char)
	}

	// A line with only man page bold caps should be considered a heading
	for _, token := range StyledRunesFromString(twin.StyleDefault, manPageHeading, nil, 0).StyledRunes {
		assert.Equal(t, token.Style, ManPageHeading)
	}

	// A line with only non-man-page bold caps should not be considered a heading
	wrongKindOfBold := "\x1b[1mJOHAN HELLO"
	for _, token := range StyledRunesFromString(twin.StyleDefault, wrongKindOfBold, nil, 0).StyledRunes {
		assert.Equal(t, token.Style, twin.StyleDefault.WithAttr(twin.AttrBold))
	}

	// A line with not all caps should not be considered a heading
	for _, token := range StyledRunesFromString(twin.StyleDefault, notAllCaps, nil, 0).StyledRunes {
		assert.Equal(t, token.Style, twin.StyleDefault.WithAttr(twin.AttrBold))
	}
}

func TestConsumeCompositeColorHappy(t *testing.T) {
	// 8 bit color
	// Example from: https://github.com/walles/moor/issues/14
	newIndex, color, err := consumeCompositeColor([]uint{38, 5, 74}, 0)
	assert.NilError(t, err)
	assert.Equal(t, newIndex, 3)
	assert.Equal(t, *color, twin.NewColor256(74))

	// 24 bit color
	newIndex, color, err = consumeCompositeColor([]uint{38, 2, 10, 20, 30}, 0)
	assert.NilError(t, err)
	assert.Equal(t, newIndex, 5)
	assert.Equal(t, *color, twin.NewColor24Bit(10, 20, 30))
}

func TestConsumeCompositeColorBadPrefix(t *testing.T) {
	// 8 bit color
	// Example from: https://github.com/walles/moor/issues/14
	_, color, err := consumeCompositeColor([]uint{29}, 0)
	assert.Equal(t, err.Error(), "unknown start of color sequence <29>, expected 38 (foreground), 48 (background) or 58 (underline): <CSI 29m>")
	assert.Assert(t, color == nil)
}

func TestConsumeCompositeColorBadType(t *testing.T) {
	_, color, err := consumeCompositeColor([]uint{38, 4}, 0)
	// https://en.wikipedia.org/wiki/ANSI_escape_code#Colors
	assert.Equal(t, err.Error(), "unknown color type <4>, expected 5 (8 bit color) or 2 (24 bit color): <CSI 38;4m>")
	assert.Assert(t, color == nil)
}

func TestConsumeCompositeColorIncomplete(t *testing.T) {
	_, color, err := consumeCompositeColor([]uint{38}, 0)
	assert.Equal(t, err.Error(), "incomplete color sequence: <CSI 38m>")
	assert.Assert(t, color == nil)
}

func TestConsumeCompositeColorIncomplete8Bit(t *testing.T) {
	_, color, err := consumeCompositeColor([]uint{38, 5}, 0)
	assert.Equal(t, err.Error(), "incomplete 8 bit color sequence: <CSI 38;5m>")
	assert.Assert(t, color == nil)
}

func TestConsumeCompositeColorIncomplete24Bit(t *testing.T) {
	_, color, err := consumeCompositeColor([]uint{38, 2, 10, 20}, 0)
	assert.Equal(t, err.Error(), "incomplete 24 bit color sequence, expected N8;2;R;G;Bm: <CSI 38;2;10;20m>")
	assert.Assert(t, color == nil)
}

func TestRawUpdateStyle(t *testing.T) {
	numberColored, _, err := rawUpdateStyle(twin.StyleDefault, "33m", make([]uint, 0))
	assert.NilError(t, err)
	assert.Equal(t, numberColored, twin.StyleDefault.WithForeground(twin.NewColor16(3)))
}

// Test with the recommended terminator ESC-backslash.
//
// Ref: https://gist.github.com/egmontkob/eb114294efbcd5adb1944c9f3cb5feda#the-escape-sequence
func TestHyperlink_escBackslash(t *testing.T) {
	url := "http://example.com"

	tokens := StyledRunesFromString(twin.StyleDefault, "a\x1b]8;;"+url+"\x1b\\bc\x1b]8;;\x1b\\d", nil, 0).StyledRunes

	assert.DeepEqual(t, tokens, []CellWithMetadata{
		{Rune: 'a', Style: twin.StyleDefault},
		{Rune: 'b', Style: twin.StyleDefault.WithHyperlink(&url)},
		{Rune: 'c', Style: twin.StyleDefault.WithHyperlink(&url)},
		{Rune: 'd', Style: twin.StyleDefault},
	},
		cmp.Comparer(func(a, b CellWithMetadata) bool { return a.Equal(b) }))
}

// Test with the not-recommended terminator BELL (ASCII 7).
//
// Ref: https://gist.github.com/egmontkob/eb114294efbcd5adb1944c9f3cb5feda#the-escape-sequence
func TestHyperlink_bell(t *testing.T) {
	url := "http://example.com"

	tokens := StyledRunesFromString(twin.StyleDefault, "a\x1b]8;;"+url+"\x07bc\x1b]8;;\x07d", nil, 0).StyledRunes

	assert.DeepEqual(t, tokens, []CellWithMetadata{
		{Rune: 'a', Style: twin.StyleDefault},
		{Rune: 'b', Style: twin.StyleDefault.WithHyperlink(&url)},
		{Rune: 'c', Style: twin.StyleDefault.WithHyperlink(&url)},
		{Rune: 'd', Style: twin.StyleDefault},
	},
		cmp.Comparer(func(a, b CellWithMetadata) bool { return a.Equal(b) }))
}

// Test with some other ESC sequence than ESC-backslash
func TestHyperlink_nonTerminatingEsc(t *testing.T) {
	complete := "a\x1b]8;;https://example.com\x1bbc"
	tokens := StyledRunesFromString(twin.StyleDefault, complete, nil, 0).StyledRunes

	// This should not be treated as any link
	for i := 0; i < len(complete); i++ {
		if complete[i] == '\x1b' {
			// These get special rendering, if everything else matches that's
			// good enough.
			continue
		}
		assert.Equal(t, tokens[i], CellWithMetadata{Rune: rune(complete[i]), Style: twin.StyleDefault},
			"i=%d, c=%s, tokens=%v", i, string(complete[i]), tokens)
	}
}

func TestHyperlink_incomplete(t *testing.T) {
	complete := "a\x1b]8;;X\x1b\\"

	for l := len(complete) - 1; l >= 0; l-- {
		incomplete := complete[:l]
		t.Run(fmt.Sprintf("l=%d incomplete=<%s>", l, strings.ReplaceAll(incomplete, "\x1b", "ESC")), func(t *testing.T) {
			tokens := StyledRunesFromString(twin.StyleDefault, incomplete, nil, 0).StyledRunes

			for i := 0; i < l; i++ {
				if complete[i] == '\x1b' {
					// These get special rendering, if everything else matches
					// that's good enough.
					continue
				}
				assert.Equal(t, tokens[i], CellWithMetadata{Rune: rune(complete[i]), Style: twin.StyleDefault})
			}
		})
	}
}

func TestRawUpdateStyleResetDoesNotAffectHyperlink(t *testing.T) {
	url := "file:///Users/johan/src/riff/src/refiner.rs"
	styleWithLink := twin.StyleDefault.WithHyperlink(&url)

	// ESC[m should reset style, but not touch the hyperlink
	updated, _, err := rawUpdateStyle(styleWithLink, "m", nil)
	assert.NilError(t, err)
	assert.Assert(t, updated.HyperlinkURL() != nil)
	assert.Equal(t, *updated.HyperlinkURL(), url)
}

// Ref: https://github.com/walles/moor/issues/372
func TestIssue372(t *testing.T) {
	const maxTokensCount = 10

	// Load test data once
	data, err := os.ReadFile(path.Join(samplesDir, "issue-372.txt"))
	assert.NilError(t, err)

	// Expect one newline terminated line
	lines := strings.Split(string(data), "\n")
	assert.Equal(t, 2, len(lines))
	assert.Equal(t, 0, len(lines[1]))

	styled := StyledRunesFromString(twin.StyleDefault, lines[0], nil, maxTokensCount).StyledRunes
	assert.Equal(t, len(styled), maxTokensCount)
}

// Benchmark stripping formatting from a colored git diff sample.
// To run:
//
//	go test -bench=BenchmarkStripFormatting -benchmem ./...
func BenchmarkStripFormatting(b *testing.B) {
	// Load sample input once
	data, err := os.ReadFile(path.Join(samplesDir, "gitdiff-color.txt"))
	if err != nil {
		b.Fatalf("read sample: %v", err)
	}

	lines := strings.Split(string(data), "\n")
	// Set processed bytes per iteration
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, line := range lines {
			// We ignore the output; benchmarking execution time only
			_ = StripFormatting(line, linemetadata.Index{}, 0)
		}
	}
}

// Benchmark stripping formatting from a colored git diff sample.
// To run:
//
//	go test -bench=BenchmarkStripFormattingUnformattedInput -benchmem ./...
func BenchmarkStripFormattingUnformattedInput(b *testing.B) {
	// Load sample input once
	data, err := os.ReadFile(path.Join(samplesDir, "gitdiff-color.txt"))
	if err != nil {
		b.Fatalf("read sample: %v", err)
	}

	// Remove formatting before benchmarking
	var unformatted strings.Builder
	formattedLines := strings.Split(string(data), "\n")
	for _, line := range formattedLines {
		unformatted.WriteString(StripFormatting(line, linemetadata.Index{}, 0))
		unformatted.WriteString("\n")
	}

	lines := strings.Split(unformatted.String(), "\n")
	// Set processed bytes per iteration
	b.SetBytes(int64(len(unformatted.String())))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, line := range lines {
			// We ignore the output; benchmarking execution time only
			_ = StripFormatting(line, linemetadata.Index{}, 0)
		}
	}
}
