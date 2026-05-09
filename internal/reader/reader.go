package reader

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"math"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/walles/moor/v2/internal/linemetadata"
	"github.com/walles/moor/v2/internal/util"

	"github.com/alecthomas/chroma/v2"
	log "github.com/sirupsen/logrus"
)

// An 1.7MB file took 2s to highlight. The number for this limit is totally
// negotiable.
const MAX_HIGHLIGHT_SIZE int64 = 2_000_000

// To cap resource usage when not needed, start by reading this many lines into
// memory. If the user scrolls near the end or starts searching, we'll read
// more.
//
// Ref: https://github.com/walles/moor/issues/296
const DEFAULT_PAUSE_AFTER_LINES = 50_000

var DisablePlainCachingForBenchmarking = false

type ReaderOptions struct {
	// Format JSON input
	ShouldFormat bool

	// Pause after reading this many lines, unless told otherwise.
	// Tune at runtime using SetPauseAfterLines().
	//
	// nil means 20k lines.
	PauseAfterLines *int

	// If this is nil, you must call reader.SetStyleForHighlighting() later if
	// you want highlighting.
	Style *chroma.Style

	// If this is set, it will be used as the lexer for highlighting
	Lexer chroma.Lexer
}

type Reader interface {
	GetLineCount() int
	GetLine(index linemetadata.Index) *NumberedLine

	// This method will try to honor wantedLineCount over firstLine. This means
	// that the returned first line may be different from the requested one.
	GetLines(firstLine linemetadata.Index, wantedLineCount int) InputLines

	// GetLines gets the indicated lines from the input. The lines will be stored
	// in the provided preallocated slice to avoid allocations. The line count is
	// determined by the capacity of the provided slice.
	//
	// The return value is the status text for the returned lines.
	GetLinesPreallocated(firstLine linemetadata.Index, resultLines *[]NumberedLine) (string, string)

	// False when paused. Showing the paused line count is confusing, because
	// the user might think that the number is the total line count, even though
	// we are not done yet.
	//
	// When we're not paused, the number will be constantly changing, indicating
	// that the counting is not done yet.
	ShouldShowLineCount() bool
}

// ReaderImpl reads a file into an array of strings.
//
// It does the reading in the background, and it returns parts of the read data
// upon request.
//
// This package provides query methods for the struct, no peeking!!
type ReaderImpl struct {
	sync.RWMutex

	lines []*Line

	// Display name for the buffer. If not set, no buffer name will be shown.
	//
	// For files, this will be the basename of the file. For our help text, this
	// will be "Help". For streams this will generally not be set, but may come
	// from the $PAGER_LABEL environment variable.
	DisplayName *string

	// If this is set, it will point out the file we are reading from. If this
	// is not set, we are not reading from a file.
	FileName *string

	// True if the file we read from was compressed.
	IsCompressed bool

	// How many bytes have we successfully decoded and read into memory so far?
	//
	// Note: For compressed files, this is the DECOMPRESSED byte count. Do NOT
	// compare this directly to os.FileInfo.Size() (which is the compressed size
	// on disk), as they represent different metrics.
	bytesCount int64

	// The first bytes read from the file. Used to determine if the file was replaced
	// or appended to when it grows.
	headerBytes []byte

	endsWithNewline bool

	Err error

	// Stream has been completely read. May not be highlighted yet.
	ReadingDone *atomic.Bool

	// Highlighting has been completed.
	HighlightingDone *atomic.Bool

	highlightingStyle chan chroma.Style

	// This channel expects to be read exactly once. All other uses will lead to
	// undefined behavior.
	doneWaitingForFirstByte chan bool

	// Used for detecting file modifications
	lastStat os.FileInfo

	// For telling the UI it should recheck the --quit-if-one-screen conditions.
	// Signalled when either highlighting is done or reading is done.
	MaybeDone chan bool

	MoreLinesAdded chan bool

	// Because we don't want to consume infinitely.
	//
	// Ref: https://github.com/walles/moor/issues/296
	pauseAfterLines        int
	pauseAfterLinesUpdated chan bool

	// PauseStatus is true if the reader is paused, false if it is not
	PauseStatus *atomic.Bool

	// Stored for use when reloading the file after it has been rewritten.
	formatter     chroma.Formatter
	readerOptions ReaderOptions

	// Set to true when this reader is discarded.
	closed atomic.Bool
}

// InputLines contains a number of lines from the reader, plus metadata
type InputLines struct {
	Lines []NumberedLine

	// "monkey.txt: 1-23/45 51%"
	FilenameText string
	StatusText   string
}

func TryOpen(filename string) error {
	// Try opening the file
	tryMe, err := os.Open(filename)
	if err != nil {
		return err
	}

	fileInfo, err := tryMe.Stat()
	if err != nil {
		closeErr := tryMe.Close()
		if closeErr != nil {
			return fmt.Errorf("failed to close %s after stat error: %w", filename, closeErr)
		}

		return err
	}

	if fileInfo.IsDir() {
		closeErr := tryMe.Close()
		if closeErr != nil {
			return fmt.Errorf("failed to close directory %s: %w", filename, closeErr)
		}

		return fmt.Errorf("%s is a directory", filename)
	}

	closeErr := tryMe.Close()
	if closeErr != nil {
		// Everything worked up until Close(), report the Close() error
		return closeErr
	}

	return nil
}

// From: https://stackoverflow.com/a/52153000/473672
func countLines(filename string) (uint64, error) {
	const lineBreak = '\n'
	sliceWithSingleLineBreak := []byte{lineBreak}

	reader, _, err := ZOpen(filename)
	if err != nil {
		return 0, err
	}
	defer func() {
		err := reader.Close()
		if err != nil {
			log.Warn("Error closing file after counting the lines: ", err)
		}
	}()

	var count uint64
	t0 := time.Now()
	buf := make([]byte, bufio.MaxScanTokenSize)
	lastReadEndsInNewline := true
	for {
		bufferSize, err := reader.Read(buf)
		if err != nil && err != io.EOF {
			return 0, err
		}

		if bufferSize > 0 {
			lastReadEndsInNewline = (buf[bufferSize-1] == lineBreak)
		}

		count += uint64(bytes.Count(buf[:bufferSize], sliceWithSingleLineBreak))
		if err == io.EOF {
			break
		}
	}

	if !lastReadEndsInNewline {
		// No trailing line feed, this needs special handling
		count++
	}

	t1 := time.Now()
	if count == 0 {
		log.Debug("Counted ", count, " lines in ", t1.Sub(t0))
	} else {
		log.Debug("Counted ", count, " lines in ", t1.Sub(t0), " at ", t1.Sub(t0)/time.Duration(count), "/line")
	}
	return count, nil
}

// Wait for reader to finish reading and highlighting. Used by tests.
func (reader *ReaderImpl) Wait() error {
	// Wait for our goroutine to finish
	for !reader.ReadingDone.Load() {
		if reader.PauseStatus.Load() {
			// We want more lines
			reader.SetPauseAfterLines(reader.GetLineCount() * 2)
		}
	}
	//revive:disable-next-line:empty-block
	for !reader.HighlightingDone.Load() {
	}

	reader.RLock()
	defer reader.RUnlock()
	return reader.Err
}

// createStatusUnlocked() assumes that its caller is holding the read lock
func (reader *ReaderImpl) createStatusUnlocked(lastLine linemetadata.Index) (string, string) {
	displayName := ""
	if reader.DisplayName != nil {
		displayName = *reader.DisplayName
	}

	if len(reader.lines) == 0 {
		empty := "<empty>"
		if len(displayName) > 0 {
			return displayName, ": " + empty
		}
		return "", empty
	}

	linesCount := ""
	percent := ""
	if len(reader.lines) == 1 {
		linesCount = "1 line"
		percent = "100%"
	} else {
		// More than one line
		linesCount = util.FormatInt(len(reader.lines)) + " lines"
		percent = fmt.Sprintf("%.0f%%", math.Floor(100*float64(lastLine.Index()+1)/float64(len(reader.lines))))
	}

	if !reader.ShouldShowLineCount() {
		linesCount = ""
	}

	return_me := ""

	if len(linesCount) > 0 {
		if len(displayName) > 0 {
			return_me += ": "
		}
		return_me += linesCount
	}

	if len(percent) > 0 {
		if len(return_me) > 0 {
			return_me += "  "
		}
		return_me += percent
	}

	if len(displayName) > 0 {
		return displayName, return_me
	}
	return "", return_me

}

// Wait for the first line to be read.
//
// Used for making sudo work:
// https://github.com/walles/moor/issues/199
func (reader *ReaderImpl) AwaitFirstByte() {
	<-reader.doneWaitingForFirstByte
}

// GetLineCount returns the number of lines available for viewing
func (reader *ReaderImpl) GetLineCount() int {
	reader.RLock()
	defer reader.RUnlock()

	return len(reader.lines)
}

func (reader *ReaderImpl) ShouldShowLineCount() bool {
	if reader.ReadingDone.Load() {
		// We are done, the number won't change, show it!
		return true
	}

	if !reader.PauseStatus.Load() {
		// Reading in progress, number is constantly changing so it's
		// obvious we aren't done yet. Show it!
		return true
	}

	return false
}

// GetLine gets a line. If the requested line number is out of bounds, nil is returned.
func (reader *ReaderImpl) GetLine(index linemetadata.Index) *NumberedLine {
	reader.RLock()

	if index.Index() >= reader.pauseAfterLines-DEFAULT_PAUSE_AFTER_LINES/2 {
		// Switch to the write lock for changing the pause threshold
		reader.RUnlock()
		reader.Lock()

		// Getting close(ish) to the pause threshold, bump it up. The Max()
		// construct is to handle the case when the add overflows.
		reader.pauseAfterLines = slices.Max([]int{
			reader.pauseAfterLines + DEFAULT_PAUSE_AFTER_LINES/2,
			reader.pauseAfterLines})

		select {
		case reader.pauseAfterLinesUpdated <- true:
		default:
			// Default case required for the write to be non-blocking
		}

		// Back to read lock for the rest of this function
		reader.Unlock()
		reader.RLock()
	}

	if !index.IsWithinLength(len(reader.lines)) {
		reader.RUnlock()
		return nil
	}

	returnLine := reader.lines[index.Index()]
	reader.RUnlock()

	return &NumberedLine{
		Index:  index,
		Number: linemetadata.NumberFromZeroBased(index.Index()),
		Line:   returnLine,
	}
}

// Given a starting point and a count, return a start and end index that don't
// exceed maxIndex. On overflow, the requested range will be shifted backwards
// to fit within maxIndex, and if that's not enough, cut at the end.
func clipRangeToLength(start linemetadata.Index, wantedCount int, maxIndex int) (int, int) {
	if wantedCount <= 0 {
		panic(fmt.Sprintf("wantedCount must be at least 1, was %d", wantedCount))
	}
	if maxIndex < 0 {
		panic(fmt.Sprintf("maxIndex must be at least 0, was %d", maxIndex))
	}

	first := start.Index()

	// Cap wantedCount to the available length (maxIndex+1).
	available := maxIndex + 1
	if wantedCount > available {
		wantedCount = available
	}

	// Clamp start so the window fits: start <= maxIndex - (wantedCount - 1)
	highestStart := maxIndex - (wantedCount - 1)
	if first > highestStart {
		first = highestStart
	}
	if first < 0 {
		first = 0
	}

	last := first + wantedCount - 1
	if last > maxIndex {
		last = maxIndex
	}

	return first, last
}

// GetLines gets the indicated lines from the input
func (reader *ReaderImpl) GetLines(firstLine linemetadata.Index, wantedLineCount int) InputLines {
	reader.RLock()
	lineCount := len(reader.lines)
	if lineCount == 0 || wantedLineCount == 0 {
		filenameText, statusText := reader.createStatusUnlocked(firstLine)
		reader.RUnlock()

		return InputLines{
			FilenameText: filenameText,
			StatusText:   statusText,
		}
	}
	reader.RUnlock()

	firstLineIndex, lastLineIndex := clipRangeToLength(firstLine, wantedLineCount, lineCount-1)
	wantedLineCount = lastLineIndex - firstLineIndex + 1

	resultLines := make([]NumberedLine, 0, wantedLineCount)
	filenameText, statusText := reader.GetLinesPreallocated(linemetadata.IndexFromZeroBased(firstLineIndex), &resultLines)

	return InputLines{
		Lines:        resultLines,
		FilenameText: filenameText,
		StatusText:   statusText,
	}
}

// GetLines gets the indicated lines from the input. The lines will be stored
// in the provided preallocated slice to avoid allocations. The line count is
// determined by the capacity of the provided slice.
//
// The return value is the status text for the returned lines.
func (reader *ReaderImpl) GetLinesPreallocated(firstLine linemetadata.Index, resultLines *[]NumberedLine) (string, string) {
	// Clear the result slice
	*resultLines = (*resultLines)[:0]

	reader.RLock()

	if len(reader.lines) == 0 || cap(*resultLines) == 0 {
		filenameText, statusText := reader.createStatusUnlocked(firstLine)
		reader.RUnlock()

		return filenameText, statusText
	}

	// Prevent reading past the end of the available lines
	firstLineIndex, lastLineIndex := clipRangeToLength(firstLine, cap(*resultLines), len(reader.lines)-1)

	filenameText, statusText := reader.createStatusUnlocked(linemetadata.IndexFromZeroBased(lastLineIndex))

	for loopIndex, returnLine := range reader.lines[firstLineIndex : lastLineIndex+1] {
		*resultLines = append(*resultLines, NumberedLine{
			Index:  linemetadata.IndexFromZeroBased(firstLineIndex + loopIndex),
			Number: linemetadata.NumberFromZeroBased(firstLineIndex + loopIndex),
			Line:   returnLine,
		})
	}

	reader.RUnlock()

	return filenameText, statusText
}

func (reader *ReaderImpl) PumpToStdout() {
	const wantedLineCount = 100
	firstNotPrintedLine := linemetadata.Index{}

	drainLines := func() bool {
		lines := reader.GetLines(firstNotPrintedLine, wantedLineCount)
		var firstReturnedIndex linemetadata.Index
		if len(lines.Lines) > 0 {
			firstReturnedIndex = lines.Lines[0].Index
		}

		// Print the lines we got
		printed := false
		for loopIndex, line := range lines.Lines {
			lineIndex := firstReturnedIndex.NonWrappingAdd(loopIndex)
			if lineIndex.IsBefore(firstNotPrintedLine) {
				continue
			}

			fmt.Println(string(line.Line.raw))
			printed = true
			firstNotPrintedLine = lineIndex.NonWrappingAdd(1)
		}

		return printed
	}

	drainAllLines := func() {
		for drainLines() {
			// Loop here until nothing was printed
		}
	}

	done := false
	for !done {
		drainAllLines()

		select {
		case <-reader.MoreLinesAdded:
			continue
		case <-reader.MaybeDone:
			done = true
		}
	}

	// Print any remaining lines
	drainAllLines()
}

// Replace reader contents with the given text. Consider setting
// HighlightingDone and signalling the MaybeDone channel afterwards.
func (reader *ReaderImpl) setText(text string) {
	lines := []*Line{}
	for _, lineString := range strings.Split(text, "\n") {
		line := Line{raw: []byte(lineString)}
		lines = append(lines, &line)
	}

	if len(lines) > 0 && strings.HasSuffix(text, "\n") {
		// Input ends with an empty line. This makes our line count be
		// off-by-one, fix that!
		lines = lines[0 : len(lines)-1]
	}

	reader.Lock()
	reader.lines = lines
	reader.Unlock()

	log.Trace("Reader done, contents explicitly set")

	select {
	case reader.MoreLinesAdded <- true:
	default:
	}
}

func (reader *ReaderImpl) setPauseStatus(paused bool) {
	if !reader.PauseStatus.CompareAndSwap(!paused, paused) {
		// Pause status already had that value, we're done
		return
	}

	log.Debugf("Reader pause status changed to %t", paused)
}

func (reader *ReaderImpl) SetPauseAfterLines(lines int) {
	if lines < 0 {
		log.Warnf("Tried to set pause-after-lines to %d, ignoring", lines)
		return
	}

	log.Trace("Setting pause-after-lines to ", lines, "...")

	reader.Lock()
	reader.pauseAfterLines = lines
	reader.Unlock()

	// Notify the reader that the pause-after-lines value has been updated. Will
	// be noticed in the maybePause() function.
	select {
	case reader.pauseAfterLinesUpdated <- true:
	default:
		// Default case required for the write to be non-blocking
	}
}

func (reader *ReaderImpl) SetStyleForHighlighting(style chroma.Style) {
	reader.highlightingStyle <- style
}

// Close stops background routines and prevents further reading.
func (reader *ReaderImpl) Close() {
	reader.closed.Store(true)

	// Unblock any active pause
	reader.SetPauseAfterLines(math.MaxInt)
}
