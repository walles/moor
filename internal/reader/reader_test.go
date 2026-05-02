package reader

import (
	"math"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	log "github.com/sirupsen/logrus"
	"gotest.tools/v3/assert"

	"github.com/walles/moor/v2/internal/linemetadata"
)

const samplesDir = "../../sample-files"

func init() {
	// Info logs clutter at least benchmark output
	log.SetLevel(log.WarnLevel)
}

func testGetLineCount(t *testing.T, reader *ReaderImpl) {
	if strings.Contains(*reader.DisplayName, "compressed") {
		// We are no good at counting lines of compressed files, never mind
		return
	}

	cmd := exec.Command("wc", "-l", *reader.FileName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Error("Error calling wc -l to count lines of", *reader.FileName, err)
	}

	wcNumberString := strings.Split(strings.TrimSpace(string(output)), " ")[0]
	wcLineCount, err := strconv.Atoi(wcNumberString)
	if err != nil {
		t.Error("Error counting lines of", *reader.FileName, err)
	}

	// wc -l under-counts by 1 if the file doesn't end in a newline
	rawBytes, err := os.ReadFile(*reader.FileName)
	if err == nil && len(rawBytes) > 0 && rawBytes[len(rawBytes)-1] != '\n' {
		wcLineCount++
	}

	if reader.GetLineCount() != wcLineCount {
		t.Errorf("Got %d lines from the reader but %d lines from wc -l: <%s>",
			reader.GetLineCount(), wcLineCount, *reader.FileName)
	}

	countLinesCount, err := countLines(*reader.FileName)
	assert.NilError(t, err)
	if countLinesCount != uint64(wcLineCount) {
		t.Errorf("Got %d lines from wc -l, but %d lines from our countLines() function", wcLineCount, countLinesCount)
	}
}

func firstLine(inputLines InputLines) linemetadata.Index {
	return inputLines.Lines[0].Index
}

func testGetLines(t *testing.T, reader *ReaderImpl) {
	lines := reader.GetLines(linemetadata.Index{}, 10)
	if len(lines.Lines) > 10 {
		t.Errorf("Asked for 10 lines, got too many: %d", len(lines.Lines))
	}

	if len(lines.Lines) < 10 {
		// No good plan for how to test short files, more than just
		// querying them, which we just did
		return
	}

	// Test clipping at the end
	lines = reader.GetLines(linemetadata.IndexMax(), 10)
	if len(lines.Lines) != 10 {
		t.Errorf("Asked for 10 lines but got %d", len(lines.Lines))
		return
	}

	startOfLastSection := firstLine(lines)
	lines = reader.GetLines(startOfLastSection, 10)
	if firstLine(lines) != startOfLastSection {
		t.Errorf("Expected start line %d when asking for the last 10 lines, got %s",
			startOfLastSection, firstLine(lines).Format())
		return
	}
	if len(lines.Lines) != 10 {
		t.Errorf("Expected 10 lines when asking for the last 10 lines, got %d",
			len(lines.Lines))
		return
	}

	lines = reader.GetLines(startOfLastSection.NonWrappingAdd(1), 10)
	if firstLine(lines) != startOfLastSection {
		t.Errorf("Expected start line %d when asking for the last+1 10 lines, got %s",
			startOfLastSection, firstLine(lines).Format())
		return
	}
	if len(lines.Lines) != 10 {
		t.Errorf("Expected 10 lines when asking for the last+1 10 lines, got %d",
			len(lines.Lines))
		return
	}

	lines = reader.GetLines(startOfLastSection.NonWrappingAdd(-1), 10)
	if firstLine(lines) != startOfLastSection.NonWrappingAdd(-1) {
		t.Errorf("Expected start line %d when asking for the last-1 10 lines, got %s",
			startOfLastSection, firstLine(lines).Format())
		return
	}
	if len(lines.Lines) != 10 {
		t.Errorf("Expected 10 lines when asking for the last-1 10 lines, got %d",
			len(lines.Lines))
		return
	}
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

func TestGetLines(t *testing.T) {
	for _, file := range getTestFiles(t) {
		t.Run(file, func(t *testing.T) {
			reader, err := NewFromFilename(file, formatters.TTY16m, ReaderOptions{Style: styles.Get("native")})
			if err != nil {
				t.Errorf("Error opening file <%s>: %s", file, err.Error())
				return
			}
			if err := reader.Wait(); err != nil {
				t.Errorf("Error reading file <%s>: %s", file, err.Error())
				return
			}

			t.Run(file, func(t *testing.T) {
				testGetLines(t, reader)
				testGetLineCount(t, reader)
				testHighlightingLineCount(t, file)
			})
		})
	}
}

func testHighlightingLineCount(t *testing.T, filenameWithPath string) {
	// This won't work on compressed files
	if strings.HasSuffix(filenameWithPath, ".xz") {
		return
	}
	if strings.HasSuffix(filenameWithPath, ".bz2") {
		return
	}
	if strings.HasSuffix(filenameWithPath, ".gz") {
		return
	}
	if strings.HasSuffix(filenameWithPath, ".zst") {
		return
	}
	if strings.HasSuffix(filenameWithPath, ".zstd") {
		return
	}

	// Load the unformatted file
	rawBytes, err := os.ReadFile(filenameWithPath)
	assert.NilError(t, err)
	rawContents := string(rawBytes)

	// Count its lines
	rawLinefeedsCount := strings.Count(rawContents, "\n")
	rawRunes := []rune(rawContents)
	rawFileEndsWithNewline := true // Special case empty files
	if len(rawRunes) > 0 {
		rawFileEndsWithNewline = rawRunes[len(rawRunes)-1] == '\n'
	}
	rawLinesCount := rawLinefeedsCount
	if !rawFileEndsWithNewline {
		rawLinesCount++
	}

	// Then load the same file using one of our Readers
	reader, err := NewFromFilename(filenameWithPath, formatters.TTY16m, ReaderOptions{Style: styles.Get("native")})
	assert.NilError(t, err)
	err = reader.Wait()
	assert.NilError(t, err)

	highlightedLinesCount := reader.GetLineCount()
	assert.Equal(t, rawLinesCount, highlightedLinesCount)
}

func TestGetLongLine(t *testing.T) {
	file := samplesDir + "/very-long-line.txt"
	reader, err := NewFromFilename(file, formatters.TTY16m, ReaderOptions{Style: styles.Get("native")})
	assert.NilError(t, err)
	assert.NilError(t, reader.Wait())

	lines := reader.GetLines(linemetadata.Index{}, 5)
	assert.Equal(t, firstLine(lines), linemetadata.Index{})
	assert.Equal(t, len(lines.Lines), 1)

	line := lines.Lines[0]
	assert.Assert(t, strings.HasPrefix(line.Plain(), "1 2 3 4"), "<%s>", line.Plain())
	assert.Assert(t, strings.HasSuffix(line.Plain(), "0123456789"), line.Plain())

	assert.Equal(t, len(line.Plain()), 100021)
}

func getReaderWithLineCount(totalLines int) *ReaderImpl {
	return NewFromTextForTesting("", strings.Repeat("x\n", totalLines))
}

func testStatusText(t *testing.T, fromLine linemetadata.Index, toLine linemetadata.Index, totalLines int, expected string) {
	testMe := getReaderWithLineCount(totalLines)
	linesRequested := fromLine.CountLinesTo(toLine)
	lines := testMe.GetLines(fromLine, linesRequested)
	statusText := lines.StatusText
	assert.Equal(t, statusText, expected)
}

func TestStatusText(t *testing.T) {
	testStatusText(t, linemetadata.Index{}, linemetadata.IndexFromOneBased(10), 20, "20 lines  50%")
	testStatusText(t, linemetadata.Index{}, linemetadata.IndexFromOneBased(5), 5, "5 lines  100%")
	testStatusText(t,
		linemetadata.IndexFromOneBased(998),
		linemetadata.IndexFromOneBased(999),
		1000,
		"1000 lines  99%")

	testStatusText(t, linemetadata.Index{}, linemetadata.Index{}, 0, "<empty>")
	testStatusText(t, linemetadata.Index{}, linemetadata.Index{}, 1, "1 line  100%")

	// Test with filename
	testMe, err := NewFromFilename(samplesDir+"/empty", formatters.TTY16m, ReaderOptions{Style: styles.Get("native")})
	assert.NilError(t, err)
	assert.NilError(t, testMe.Wait())

	line := testMe.GetLines(linemetadata.Index{}, 0)
	if line.Lines != nil {
		t.Error("line.lines is should have been nil when reading from an empty stream")
	}
	assert.Equal(t, line.FilenameText, "empty")
	assert.Equal(t, line.StatusText, ": <empty>")
}

func testCompressedFile(t *testing.T, filename string) {
	filenameWithPath := path.Join(samplesDir, filename)
	reader, e := NewFromFilename(filenameWithPath, formatters.TTY16m, ReaderOptions{Style: styles.Get("native")})
	if e != nil {
		t.Errorf("Error opening file <%s>: %s", filenameWithPath, e.Error())
		panic(e)
	}
	assert.NilError(t, reader.Wait())

	lines := reader.GetLines(linemetadata.Index{}, 5)
	assert.Equal(t, lines.Lines[0].Plain(), "This is a compressed file", "%s", filename)
}

func TestCompressedFiles(t *testing.T) {
	testCompressedFile(t, "compressed.txt.gz")
	testCompressedFile(t, "compressed.txt.bz2")
	testCompressedFile(t, "compressed.txt.xz")
	testCompressedFile(t, "compressed.txt.zst")
	testCompressedFile(t, "compressed.txt.zstd")
}

func TestReadFileDoneNoHighlighting(t *testing.T) {
	testMe, err := NewFromFilename(samplesDir+"/empty",
		formatters.TTY, ReaderOptions{Style: styles.Get("native")})
	assert.NilError(t, err)

	assert.NilError(t, testMe.Wait())
}

func TestReadFileDoneYesHighlighting(t *testing.T) {
	testMe, err := NewFromFilename("reader_test.go",
		formatters.TTY, ReaderOptions{Style: styles.Get("native")})
	assert.NilError(t, err)

	assert.NilError(t, testMe.Wait())
}

func TestReadStreamDoneNoHighlighting(t *testing.T) {
	testMe, err := NewFromStream("", strings.NewReader("Johan"), nil, ReaderOptions{Style: &chroma.Style{}})
	assert.NilError(t, err)

	assert.NilError(t, testMe.Wait())
}

func TestReadStreamDoneYesHighlighting(t *testing.T) {
	testMe, err := NewFromStream("",
		strings.NewReader("Johan"),
		formatters.TTY, ReaderOptions{Lexer: lexers.EmacsLisp, Style: styles.Get("native")})
	assert.NilError(t, err)

	assert.NilError(t, testMe.Wait())
}

func TestReadFromDevFd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("/dev/fd is not available on Windows")

		return
	}

	readPipe, writePipe, err := os.Pipe()
	assert.NilError(t, err)
	defer readPipe.Close() //nolint:errcheck

	_, err = writePipe.WriteString("test\n")
	assert.NilError(t, err)
	assert.NilError(t, writePipe.Close())

	fileName := "/dev/fd/" + strconv.Itoa(int(readPipe.Fd()))
	testMe, err := NewFromFilename(fileName, formatters.TTY, ReaderOptions{Style: styles.Get("native")})
	assert.NilError(t, err)
	assert.NilError(t, testMe.Wait())

	lines := testMe.GetLines(linemetadata.Index{}, 10)
	assert.Equal(t, len(lines.Lines), 1)
	assert.Equal(t, lines.Lines[0].Plain(), "test")
}

func TestTryOpenDirectory(t *testing.T) {
	tempDir := t.TempDir()

	err := TryOpen(tempDir)
	assert.Assert(t, err != nil, "TryOpen should fail on directories")
}

func TestReadTextDone(t *testing.T) {
	testMe := NewFromTextForTesting("", "Johan")

	assert.NilError(t, testMe.Wait())
}

// JSON should be auto detected and formatted
func TestFormatJson(t *testing.T) {
	// Note the space after "key" to verify formatting actually happens
	jsonStream := strings.NewReader(`{"key" :"value"}`)
	testMe, err := NewFromStream(
		"JSON test",
		jsonStream,
		formatters.TTY,
		ReaderOptions{
			Style:        styles.Get("native"),
			ShouldFormat: true,
		})
	assert.NilError(t, err)

	assert.NilError(t, testMe.Wait())

	lines := testMe.GetLines(linemetadata.Index{}, 10)
	assert.Equal(t, lines.Lines[0].Plain(), "{")
	assert.Equal(t, lines.Lines[1].Plain(), `  "key": "value"`)
	assert.Equal(t, lines.Lines[2].Plain(), "}")
	assert.Equal(t, len(lines.Lines), 3)
}

func TestFormatJsonArray(t *testing.T) {
	// Note the space after "key" to verify formatting actually happens
	jsonStream := strings.NewReader(`[{"key" :"value"}]`)
	testMe, err := NewFromStream(
		"JSON test",
		jsonStream,
		formatters.TTY,
		ReaderOptions{
			Style:        styles.Get("native"),
			ShouldFormat: true,
		})
	assert.NilError(t, err)

	assert.NilError(t, testMe.Wait())

	lines := testMe.GetLines(linemetadata.Index{}, 10)
	assert.Equal(t, lines.Lines[0].Plain(), "[")
	assert.Equal(t, lines.Lines[1].Plain(), "  {")
	assert.Equal(t, lines.Lines[2].Plain(), `    "key": "value"`)
	assert.Equal(t, lines.Lines[3].Plain(), "  }")
	assert.Equal(t, lines.Lines[4].Plain(), "]")
	assert.Equal(t, len(lines.Lines), 5)
}

// If people keep appending to the currently opened file we should display those
// changes.
func TestReadUpdatingFile(t *testing.T) {
	// Make a temp file containing one line of text, ending with a newline
	file, err := os.CreateTemp("", "moor-TestReadUpdatingFile-*.txt")
	assert.NilError(t, err)
	defer os.Remove(file.Name()) //nolint:errcheck

	const firstLineString = "First line\n"
	_, err = file.WriteString(firstLineString)
	assert.NilError(t, err)

	// Start a reader on that file
	testMe, err := NewFromFilename(file.Name(), formatters.TTY16m, ReaderOptions{Style: styles.Get("native")})
	assert.NilError(t, err)

	// Wait for the reader to finish reading
	assert.NilError(t, testMe.Wait())
	assert.Equal(t, len([]byte(firstLineString)), int(testMe.bytesCount))

	// Verify we got the single line
	allLines := testMe.GetLines(linemetadata.Index{}, 10)
	assert.Equal(t, len(allLines.Lines), 1)
	assert.Equal(t, testMe.GetLineCount(), 1)
	assert.Equal(t, allLines.Lines[0].Plain(), "First line")

	// Append a line to the file
	const secondLineString = "Second line\n"
	_, err = file.WriteString(secondLineString)
	assert.NilError(t, err)

	// Give the reader some time to react
	for range 20 {
		allLines := testMe.GetLines(linemetadata.Index{}, 10)
		if len(allLines.Lines) == 2 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Verify we got the two lines
	allLines = testMe.GetLines(linemetadata.Index{}, 10)
	assert.Equal(t, len(allLines.Lines), 2, "Expected two lines after adding a second one, got %d", len(allLines.Lines))
	assert.Equal(t, testMe.GetLineCount(), 2)
	assert.Equal(t, allLines.Lines[0].Plain(), "First line")
	assert.Equal(t, allLines.Lines[1].Plain(), "Second line")

	assert.Equal(t, int(testMe.bytesCount), len([]byte(firstLineString+secondLineString)))

	// Append a third line to the file. We want to verify line 2 didn't just
	// succeed due to special handling.
	const thirdLineString = "Third line\n"
	_, err = file.WriteString(thirdLineString)
	assert.NilError(t, err)

	// Give the reader some time to react
	for i := 0; i < 20; i++ {
		allLines = testMe.GetLines(linemetadata.Index{}, 10)
		if len(allLines.Lines) == 3 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Verify we got all three lines
	allLines = testMe.GetLines(linemetadata.Index{}, 10)
	assert.Equal(t, len(allLines.Lines), 3, "Expected three lines after adding a third one, got %d", len(allLines.Lines))
	assert.Equal(t, testMe.GetLineCount(), 3)
	assert.Equal(t, allLines.Lines[0].Plain(), "First line")
	assert.Equal(t, allLines.Lines[1].Plain(), "Second line")
	assert.Equal(t, allLines.Lines[2].Plain(), "Third line")

	assert.Equal(t, int(testMe.bytesCount), len([]byte(firstLineString+secondLineString+thirdLineString)))
}

// If a file is rewritten (shrunk and replaced with new content), tailing should
// detect the shrink and reload the file rather than giving up.
func TestReadShrunkFile(t *testing.T) {
	// Make a temp file with an initial line
	file, err := os.CreateTemp("", "moor-TestReadShrunkFile-*.txt")
	assert.NilError(t, err)
	defer os.Remove(file.Name()) //nolint:errcheck

	const firstLineString = "First line\n"
	_, err = file.WriteString(firstLineString)
	assert.NilError(t, err)

	// Start a reader on that file
	testMe, err := NewFromFilename(file.Name(), formatters.TTY16m, ReaderOptions{Style: styles.Get("native")})
	assert.NilError(t, err)

	// Wait for the reader to finish reading
	assert.NilError(t, testMe.Wait())

	allLines := testMe.GetLines(linemetadata.Index{}, 10)
	assert.Equal(t, len(allLines.Lines), 1)
	assert.Equal(t, allLines.Lines[0].Plain(), "First line")

	// Rewrite the file with shorter content, so the file shrinks
	err = os.WriteFile(file.Name(), []byte("New\n"), 0600)
	assert.NilError(t, err)

	// Give the background tailing goroutine up to 2s to detect the shrink and reload
	for range 20 {
		allLines = testMe.GetLines(linemetadata.Index{}, 10)
		if len(allLines.Lines) == 1 && allLines.Lines[0].Plain() == "New" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Wait for the reload to fully complete before asserting
	assert.NilError(t, testMe.Wait())

	allLines = testMe.GetLines(linemetadata.Index{}, 10)
	assert.Equal(t, len(allLines.Lines), 1, "Expected one line after rewrite, got %d", len(allLines.Lines))
	assert.Equal(t, allLines.Lines[0].Plain(), "New")
}

func TestRequestReload(t *testing.T) {
	// Make a temp file with initial content
	file, err := os.CreateTemp("", "moor-TestRequestReload-*.txt")
	assert.NilError(t, err)
	defer os.Remove(file.Name()) //nolint:errcheck

	// Use content shorter than the replacement so the file grows on rewrite,
	// meaning auto-reload-on-shrink would not trigger.
	_, err = file.WriteString("Original\n")
	assert.NilError(t, err)

	// Start a reader on that file
	testMe, err := NewFromFilename(file.Name(), formatters.TTY16m, ReaderOptions{Style: styles.Get("native")})
	assert.NilError(t, err)

	assert.NilError(t, testMe.Wait())

	allLines := testMe.GetLines(linemetadata.Index{}, 10)
	assert.Equal(t, len(allLines.Lines), 1)
	assert.Equal(t, allLines.Lines[0].Plain(), "Original")

	// Overwrite with longer content — file grows, so auto-reload-on-shrink won't fire
	err = os.WriteFile(file.Name(), []byte("Replacement content\n"), 0600)
	assert.NilError(t, err)

	// Manually request a reload
	testMe.RequestReload()

	// Give the tailing goroutine up to 2s to reload
	for range 20 {
		allLines = testMe.GetLines(linemetadata.Index{}, 10)
		if len(allLines.Lines) == 1 && allLines.Lines[0].Plain() == "Replacement content" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Wait for the reload to fully complete before asserting
	assert.NilError(t, testMe.Wait())

	allLines = testMe.GetLines(linemetadata.Index{}, 10)
	assert.Equal(t, len(allLines.Lines), 1, "Expected one line after reload, got %d", len(allLines.Lines))
	assert.Equal(t, allLines.Lines[0].Plain(), "Replacement content")
}

// If people keep appending to the currently opened file we should display those
// changes.
//
// This test verifies it with an initially empty file.
func TestReadUpdatingFile_InitiallyEmpty(t *testing.T) {
	// Make a temp file containing one line of text, ending with a newline
	file, err := os.CreateTemp("", "moor-TestReadUpdatingFile_NoNewlineAtEOF-*.txt")
	assert.NilError(t, err)
	defer os.Remove(file.Name()) //nolint:errcheck

	// Start a reader on that file
	testMe, err := NewFromFilename(file.Name(), formatters.TTY16m, ReaderOptions{Style: styles.Get("native")})
	assert.NilError(t, err)

	// Wait for the reader to finish reading
	assert.NilError(t, testMe.Wait())

	// Verify no lines
	allLines := testMe.GetLines(linemetadata.Index{}, 10)
	assert.Equal(t, len(allLines.Lines), 0)
	assert.Equal(t, testMe.GetLineCount(), 0)

	// Append a line to the file
	_, err = file.WriteString("Text\n")
	assert.NilError(t, err)

	// Give the reader some time to react
	for i := 0; i < 20; i++ {
		allLines := testMe.GetLines(linemetadata.Index{}, 10)
		if len(allLines.Lines) == 1 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Verify we got the two lines
	allLines = testMe.GetLines(linemetadata.Index{}, 10)
	assert.Equal(t, len(allLines.Lines), 1, "Expected one line after adding one, got %d", len(allLines.Lines))
	assert.Equal(t, testMe.GetLineCount(), 1)
	assert.Equal(t, allLines.Lines[0].Plain(), "Text")
}

// If people keep appending to the currently opened file we should display those
// changes.
//
// This test verifies it with the initial contents not ending with a linefeed.
func TestReadUpdatingFile_HalfLine(t *testing.T) {
	// Make a temp file containing one line of text, ending with a newline
	file, err := os.CreateTemp("", "moor-TestReadUpdatingFile-*.txt")
	assert.NilError(t, err)
	defer os.Remove(file.Name()) //nolint:errcheck

	_, err = file.WriteString("Start")
	assert.NilError(t, err)

	// Start a reader on that file
	testMe, err := NewFromFilename(file.Name(), formatters.TTY16m, ReaderOptions{Style: styles.Get("native")})
	assert.NilError(t, err)

	// Wait for the reader to finish reading
	assert.NilError(t, testMe.Wait())
	assert.Equal(t, int(testMe.bytesCount), len([]byte("Start")))

	// Append the rest of the line
	const secondLineString = ", end\n"
	_, err = file.WriteString(secondLineString)
	assert.NilError(t, err)

	// Give the reader some time to react
	for i := 0; i < 20; i++ {
		allLines := testMe.GetLines(linemetadata.Index{}, 10)
		if len(allLines.Lines) == 2 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Verify we got the two lines
	allLines := testMe.GetLines(linemetadata.Index{}, 10)
	assert.Equal(t, len(allLines.Lines), 1, "Still expecting one line, got %d", len(allLines.Lines))
	assert.Equal(t, testMe.GetLineCount(), 1)
	assert.Equal(t, allLines.Lines[0].Plain(), "Start, end")

	assert.Equal(t, int(testMe.bytesCount), len([]byte("Start, end\n")))
}

// If people keep appending to the currently opened file we should display those
// changes.
//
// This test verifies it with the initial contents ending in the middle of an UTF-8 character.
func TestReadUpdatingFile_HalfUtf8(t *testing.T) {
	// Make a temp file containing one line of text, ending with a newline
	file, err := os.CreateTemp("", "moor-TestReadUpdatingFile-*.txt")
	assert.NilError(t, err)
	defer os.Remove(file.Name()) //nolint:errcheck

	// Write "h" and half an "ä" to the file
	_, err = file.Write([]byte("här"[0:2]))
	assert.NilError(t, err)

	// Start a reader on that file
	testMe, err := NewFromFilename(file.Name(), formatters.TTY16m, ReaderOptions{Style: styles.Get("native")})
	assert.NilError(t, err)

	// Wait for the reader to finish reading
	assert.NilError(t, testMe.Wait())
	assert.Equal(t, testMe.GetLineCount(), 1)

	// Append the rest of the UTF-8 character
	_, err = file.WriteString("här"[2:])
	assert.NilError(t, err)

	// Give the reader some time to react
	for range 20 {
		allLines := testMe.GetLines(linemetadata.Index{}, 10)
		if len(allLines.Lines) == 2 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Verify we got the two lines
	allLines := testMe.GetLines(linemetadata.Index{}, 10)
	assert.Equal(t, len(allLines.Lines), 1, "Still expecting one line, got %d", len(allLines.Lines))
	assert.Equal(t, testMe.GetLineCount(), 1)
	assert.Equal(t, allLines.Lines[0].Plain(), "här")

	assert.Equal(t, int(testMe.bytesCount), len([]byte("här")))
}

func TestClipRangeToLength(t *testing.T) {
	// Within bounds
	i0, i1 := clipRangeToLength(linemetadata.Index{}, 1, 20)
	assert.Equal(t, i0, 0)
	assert.Equal(t, i1, 0)

	// Touching the end, still within bounds
	i0, i1 = clipRangeToLength(linemetadata.Index{}, 1, 0)
	assert.Equal(t, i0, 0)
	assert.Equal(t, i1, 0)

	// Overflow, push down to indices 6, 7, 8
	i0, i1 = clipRangeToLength(linemetadata.IndexFromOneBased(100), 3, 8)
	assert.Equal(t, i0, 6)
	assert.Equal(t, i1, 8)

	// Overflow, push down and clip to indices 0, 1
	i0, i1 = clipRangeToLength(linemetadata.IndexFromOneBased(100), 3, 1)
	assert.Equal(t, i0, 0)
	assert.Equal(t, i1, 1)

	// Maxed out start
	i0, i1 = clipRangeToLength(linemetadata.IndexMax(), 1, 0)
	assert.Equal(t, i0, 0)
	assert.Equal(t, i1, 0)

	// Maxed out count
	i0, i1 = clipRangeToLength(linemetadata.Index{}, math.MaxInt, 0)
	assert.Equal(t, i0, 0)
	assert.Equal(t, i1, 0)

	// Maxed out start and count
	i0, i1 = clipRangeToLength(linemetadata.IndexMax(), math.MaxInt, 3)
	assert.Equal(t, i0, 0)
	assert.Equal(t, i1, 3)
}

// How long does it take to read a file?
//
// This can be slow due to highlighting.
//
// Run with: go test -run='^$' -bench=. . ./...
func BenchmarkReaderDone(b *testing.B) {
	filename := "reader.go" // This is our longest .go file
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		// This is our longest .go file
		readMe, err := NewFromFilename(filename, formatters.TTY16m, ReaderOptions{Style: styles.Get("native")})
		assert.NilError(b, err)

		assert.NilError(b, readMe.Wait())
		assert.NilError(b, readMe.Err)
	}
}

// Try loading a large file
func BenchmarkReadLargeFile(b *testing.B) {
	// Try loading a file this large
	const largeSizeBytes = 35_000_000

	// First, create it from something...
	inputFilename := "reader.go"
	contents, err := os.ReadFile(inputFilename)
	assert.NilError(b, err)

	testdir := b.TempDir()
	largeFileName := testdir + "/large-file"
	largeFile, err := os.Create(largeFileName)
	assert.NilError(b, err)

	totalBytesWritten := 0
	for totalBytesWritten < largeSizeBytes {
		written, err := largeFile.Write(contents)
		assert.NilError(b, err)

		totalBytesWritten += written
	}
	err = largeFile.Close()
	assert.NilError(b, err)

	// Make sure we don't pause during the benchmark
	targetLineCount := largeSizeBytes * 2

	b.SetBytes(int64(totalBytesWritten))

	// Try making the whole run more predictable
	runtime.GC()

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		readMe, err := NewFromFilename(
			largeFileName,
			formatters.TTY16m,
			ReaderOptions{
				Style:           styles.Get("native"),
				PauseAfterLines: &targetLineCount,
			})
		assert.NilError(b, err)

		<-readMe.MaybeDone
		assert.NilError(b, readMe.Wait())
		assert.NilError(b, readMe.Err)
	}
}

// Count lines in pager.go
func BenchmarkCountLines(b *testing.B) {
	// First, get some sample lines...
	inputFilename := "reader.go"
	contents, err := os.ReadFile(inputFilename)
	assert.NilError(b, err)

	testdir := b.TempDir()
	countFileName := testdir + "/count-file"
	countFile, err := os.Create(countFileName)
	assert.NilError(b, err)

	// Make a large enough test case that a majority of the time is spent
	// counting lines, rather than on any counting startup cost.
	//
	// We used to have 1000 here, but that made the benchmark result fluctuate
	// too much. 10_000 seems to provide stable enough results.
	for range 10_000 {
		_, err := countFile.Write(contents)
		assert.NilError(b, err)
	}
	err = countFile.Close()
	assert.NilError(b, err)

	b.ResetTimer()
	for range b.N {
		_, err = countLines(countFileName)
		assert.NilError(b, err)
	}
}
