package reader

import (
	"bufio"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/walles/moor/v2/internal/linemetadata"
	"github.com/walles/moor/v2/internal/textstyles"
	"github.com/walles/moor/v2/internal/util"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
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

type Line struct {
	raw            []byte
	plainTextCache atomic.Pointer[string] // Use line.Plain() to access this field
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

	// How many bytes have we read so far? -1 means unknown (non-seekable source).
	bytesCount int64

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
}

// InputLines contains a number of lines from the reader, plus metadata
type InputLines struct {
	Lines []NumberedLine

	// "monkey.txt: 1-23/45 51%"
	FilenameText string
	StatusText   string
}

// This is the reader's main function. It will be run in a goroutine. First it
// reads the stream until the end, then starts tailing.
func (reader *ReaderImpl) readStream(stream io.Reader, formatter chroma.Formatter, options ReaderOptions) {
	reader.consumeLinesFromStream(stream)

	reader.ReadingDone.Store(true)
	select {
	case reader.MaybeDone <- true:
	default:
	}

	t0 := time.Now()
	style := <-reader.highlightingStyle
	options.Style = &style

	reader.Lock()
	reader.readerOptions.Style = &style
	reader.Unlock()

	highlightFromMemory(reader, formatter, options)
	log.Debug("highlightFromMemory() took ", time.Since(t0))

	reader.HighlightingDone.Store(true)
	select {
	case reader.MaybeDone <- true:
	default:
	}

	// Tail the file if the stream is coming from a file.
	// Ref: https://github.com/walles/moor/issues/224
	err := reader.tailFile()
	if err != nil {
		log.Warn("Failed to tail file: ", err)
	}
}

// Pause if we should pause, otherwise not. Pausing means waiting for
// pauseAfterLinesUpdated to be signalled in SetPauseAfterLines().
func (reader *ReaderImpl) assumeLockAndMaybePause() {
	for {
		shouldPause := len(reader.lines) >= reader.pauseAfterLines

		if !shouldPause {
			// Not there yet, no pause
			return
		}

		// Release lock while pausing
		reader.Unlock()
		reader.setPauseStatus(true)
		<-reader.pauseAfterLinesUpdated
		reader.setPauseStatus(false)
		reader.Lock()
	}
}

// Assume write lock held. Add a new line. If this function paused, it will
// return the pause duration.
func (reader *ReaderImpl) assumeLockAndAddLine(line []byte, considerAppending bool, linePool *linePool) time.Duration {
	// Line end
	if len(line) > 0 && line[len(line)-1] == '\r' {
		line = line[:len(line)-1] // Handle MSDOS line endings
	}

	if len(reader.lines) == 0 {
		// Can't append if there are no previous lines
		considerAppending = false
	}

	if !considerAppending {
		newLine := linePool.create(line)
		reader.lines = append(reader.lines, newLine)

		// New line added, time for a break?
		t0 := time.Now()
		reader.assumeLockAndMaybePause()
		pauseDuration := time.Since(t0)

		return pauseDuration
	}

	// Special case, append to the previous line
	baseLine := reader.lines[len(reader.lines)-1]

	// Build the complete line
	completeLine := make([]byte, len(baseLine.raw)+len(line))
	copy(completeLine, baseLine.raw)
	copy(completeLine[len(baseLine.raw):], line)

	baseLine.raw = completeLine
	baseLine.plainTextCache.Store(nil) // Invalidate cache

	return 0
}

// This function will update the Reader struct. It is expected to run in a
// goroutine.
//
// It is used both during the initial read of the stream until it ends, and
// while tailing files for changes.
func (reader *ReaderImpl) consumeLinesFromStream(stream io.Reader) {
	// This value affects BenchmarkReadLargeFile() performance. Validate changes
	// like this:
	//
	//   go test -benchmem -run='^$' -bench 'BenchmarkReadLargeFile' ./internal/reader
	const byteBufferSize = 16 * 1024

	t0 := time.Now()

	// Preallocating the line pool and the lines slice improves large file
	// reading performance by 10%.
	linePool := linePool{}
	if reader.FileName != nil && reader.GetLineCount() == 0 && isSeekableFile(reader.FileName) {
		lineCount, err := countLines(*reader.FileName)
		if err != nil {
			log.Warn("Failed to count lines in file: ", err)
		} else {
			// We have a line count...
			reader.Lock()
			if len(reader.lines) == 0 {
				// ... and still no lines have been read, so preallocate both
				// the lines slice...
				reader.lines = make([]*Line, 0, lineCount)

				// ... and the line pool.
				linePool.pool = make([]Line, lineCount)
			}
			reader.Unlock()
		}
	}

	inspectionReader := inspectionReader{base: stream}

	awaitingFirstByte := true
	for {
		byteBuffer := make([]byte, byteBufferSize)
		readBytes, err := inspectionReader.Read(byteBuffer)

		if awaitingFirstByte && readBytes > 0 {
			// We got our first byte!
			select {
			case reader.doneWaitingForFirstByte <- true:
			default:
			}

			awaitingFirstByte = false
		}

		// Error or not, handle the bytes that we got
		reader.Lock()
		lineStart := 0
		byteIndex := 0
		for readBytes > 0 {
			relativeNewlineLocation := bytes.IndexByte(byteBuffer[byteIndex:readBytes], '\n')
			if relativeNewlineLocation == -1 {
				// No more newlines in this buffer
				break
			}

			byteIndex += relativeNewlineLocation

			considerAppending := lineStart == 0 && !reader.endsWithNewline
			pauseDuration := reader.assumeLockAndAddLine(byteBuffer[lineStart:byteIndex], considerAppending, &linePool)
			t0 = t0.Add(pauseDuration)

			lineStart = byteIndex + 1
			byteIndex = lineStart
		}

		// Handle any remaining bytes as a partial line
		if lineStart < readBytes {
			considerAppending := lineStart == 0 && !reader.endsWithNewline
			pauseDuration := reader.assumeLockAndAddLine(byteBuffer[lineStart:readBytes], considerAppending, &linePool)
			t0 = t0.Add(pauseDuration)
		}

		reader.endsWithNewline = inspectionReader.endedWithNewline

		reader.Unlock()

		// This is how to do a non-blocking write to a channel:
		// https://gobyexample.com/non-blocking-channel-operations
		select {
		case reader.MoreLinesAdded <- true:
		default:
			// Default case required for the write to be non-blocking
		}

		if err == io.EOF {
			// Done!
			break
		}

		if err != nil {
			reader.Lock()
			if reader.Err == nil {
				// Store the error unless it overwrites one we already have
				reader.Err = fmt.Errorf("error reading from input stream: %w", err)
			}
			reader.Unlock()
			break
		}
	}

	if reader.FileName != nil {
		reader.Lock()
		reader.bytesCount += inspectionReader.bytesCount
		reader.Unlock()
	}

	if awaitingFirstByte {
		// If the stream was empty we never got any first byte. Make sure people
		// stop waiting in this case. Async write since it might already have been
		// written to.
		select {
		case reader.doneWaitingForFirstByte <- true:
		default:
		}
	}

	log.Info("Stream read in ", time.Since(t0), ", have ", reader.GetLineCount(), " lines")
}

// reloadFromFile clears the current content and re-reads the file from scratch.
//
// FIXME: This must only be called from the tailing goroutine. If called
// concurrently with consumeLinesFromStream(), both goroutines will interleave
// line additions and bytesCount will be wrong. Fix this before adding any
// other callers (e.g. the 'r' key).
func (reader *ReaderImpl) reloadFromFile(fileName string) error {
	log.Debugf("Reloading file %s from the beginning", fileName)

	stream, _, err := ZOpen(fileName)
	if err != nil {
		return fmt.Errorf("failed to open file %s for reloading: %w", fileName, err)
	}

	reader.Lock()
	reader.lines = reader.lines[:0]
	reader.bytesCount = 0
	reader.endsWithNewline = false
	reader.Err = nil
	reader.ReadingDone.Store(false)
	reader.HighlightingDone.Store(false)
	reader.Unlock()

	// Signal the pager to redraw the now-empty content
	select {
	case reader.MoreLinesAdded <- true:
	default:
	}

	reader.consumeLinesFromStream(stream)
	err = stream.Close()
	if err != nil {
		return fmt.Errorf("failed to close file %s after reloading: %w", fileName, err)
	}

	reader.ReadingDone.Store(true)
	select {
	case reader.MaybeDone <- true:
	default:
	}

	reader.RLock()
	formatter := reader.formatter
	options := reader.readerOptions
	reader.RUnlock()

	if formatter != nil && options.Style != nil {
		highlightFromMemory(reader, formatter, options)
	}

	reader.HighlightingDone.Store(true)
	select {
	case reader.MaybeDone <- true:
	default:
	}

	return nil
}

// readNewBytes reads bytes appended to the file since we last read it.
//
// Returns (shouldContinue, error): shouldContinue=false means tailing should stop.
func (reader *ReaderImpl) readNewBytes(fileName string, bytesCount int64) (bool, error) {
	stream, _, err := ZOpen(fileName)
	if err != nil {
		log.Debugf("Failed to open file %s for re-reading while tailing: %s", fileName, err.Error())
		return false, nil
	}

	seekable, ok := stream.(io.ReadSeekCloser)
	if !ok {
		err = stream.Close()
		if err != nil {
			log.Debugf("Giving up on tailing, failed to close non-seekable stream from %s: %s", fileName, err.Error())
			return false, nil
		}
		log.Debugf("Giving up on tailing, file %s is not seekable", fileName)
		return false, nil
	}

	_, err = seekable.Seek(bytesCount, io.SeekStart)
	if err != nil {
		log.Debugf("Failed to seek in file %s while tailing: %s", fileName, err.Error())
		return false, nil
	}

	log.Tracef("File %s grew, reading more lines from byte %d...", fileName, bytesCount)

	reader.consumeLinesFromStream(seekable)
	err = seekable.Close()
	if err != nil {
		// This can lead to file handle leaks
		return false, fmt.Errorf("failed to close file %s after tailing: %w", fileName, err)
	}

	return true, nil
}

// tailOnce performs one iteration of the file tailing check.
//
// Returns (shouldContinue, error): shouldContinue=false with nil error means
// tailing should stop cleanly; shouldContinue=false with a non-nil error means
// tailing stopped due to a failure.
func (reader *ReaderImpl) tailOnce() (bool, error) {
	reader.RLock()
	fileName := reader.FileName
	reader.RUnlock()
	if fileName == nil {
		return false, nil
	}

	fileStats, err := os.Stat(*fileName)
	if err != nil {
		log.Debugf("Failed to stat file %s while tailing, giving up: %s", *fileName, err.Error())
		return false, nil
	}

	reader.RLock()
	bytesCount := reader.bytesCount
	reader.RUnlock()

	if bytesCount == -1 {
		log.Debugf("Bytes count unknown for %s, stop tailing", *fileName)
		return false, nil
	}

	if fileStats.Size() == bytesCount {
		log.Tracef("File %s unchanged at %d bytes, continue tailing", *fileName, fileStats.Size())
		return true, nil
	}

	if fileStats.Size() < bytesCount {
		err := reader.reloadFromFile(*fileName)
		if err != nil {
			return false, err
		}
		return true, nil
	}

	return reader.readNewBytes(*fileName, bytesCount)
}

func (reader *ReaderImpl) tailFile() error {
	reader.RLock()
	fileName := reader.FileName
	reader.RUnlock()
	if fileName == nil {
		return nil
	}

	if !isSeekableFile(fileName) {
		log.Debugf("Giving up on tailing, %s is not seekable", *fileName)
		return nil
	}

	log.Debugf("Tailing file %s", *fileName)

	for {
		// NOTE: We could use something like
		// https://github.com/fsnotify/fsnotify instead of sleeping and polling
		// here.
		time.Sleep(1 * time.Second)

		shouldContinue, err := reader.tailOnce()
		if err != nil {
			return err
		}
		if !shouldContinue {
			return nil
		}
	}
}

func isSeekableFile(fileName *string) bool {
	if fileName == nil {
		return false
	}

	file, err := os.Open(*fileName)
	if err != nil {
		return false
	}

	defer func() {
		err := file.Close()
		if err != nil {
			log.Debugf("Failed to close %s while checking seekability: %s", *fileName, err)
		}
	}()

	_, err = file.Seek(0, io.SeekCurrent)

	return err == nil
}

// NewFromStream creates a new stream reader
//
// The display name can be an empty string ("").
//
// If non-empty, the name will be displayed by the pager in the bottom left
// corner to help the user keep track of what is being paged.
//
// Note that you must call reader.SetStyleForHighlighting() after this to get
// highlighting.
func NewFromStream(displayName string, reader io.Reader, formatter chroma.Formatter, options ReaderOptions) (*ReaderImpl, error) {
	zReader, err := ZReader(reader)
	if err != nil {
		return nil, err
	}
	mReader := newReaderFromStream(zReader, nil, formatter, options)

	if len(displayName) > 0 {
		mReader.Lock()
		mReader.DisplayName = &displayName
		mReader.Unlock()
	}

	if options.Style != nil {
		mReader.SetStyleForHighlighting(*options.Style)
	}

	return mReader, nil
}

// newReaderFromStream creates a new stream reader
//
// originalFileName is used for counting the lines in the file. nil for
// don't-know (streams) or not countable (compressed files). The line count is
// then used for pre-allocating the lines slice, which improves large file
// loading performance.
//
// If lexer is set, the file will be highlighted after being fully read.
//
// Whatever data we get from the reader, that's what we'll have. Or in other
// words, if the input needs to be decompressed, do that before coming here.
//
// Note that you must call reader.SetStyleForHighlighting() after this to get
// highlighting.
func newReaderFromStream(reader io.Reader, originalFileName *string, formatter chroma.Formatter, options ReaderOptions) *ReaderImpl {
	readingDone := atomic.Bool{}
	readingDone.Store(false)
	highlightingDone := atomic.Bool{}
	highlightingDone.Store(false)
	pauseStatus := atomic.Bool{}
	pauseStatus.Store(false)
	pauseAfterLines := DEFAULT_PAUSE_AFTER_LINES
	if options.PauseAfterLines != nil {
		pauseAfterLines = *options.PauseAfterLines
	}
	var displayFileName *string
	if originalFileName != nil {
		basename := filepath.Base(*originalFileName)
		displayFileName = &basename
	}
	returnMe := ReaderImpl{
		FileName:    originalFileName,
		DisplayName: displayFileName,

		pauseAfterLines:        pauseAfterLines,
		pauseAfterLinesUpdated: make(chan bool, 1),

		PauseStatus: &pauseStatus,

		MoreLinesAdded:          make(chan bool, 1),
		MaybeDone:               make(chan bool, 2),
		highlightingStyle:       make(chan chroma.Style, 1),
		doneWaitingForFirstByte: make(chan bool, 1),
		HighlightingDone:        &highlightingDone,
		ReadingDone:             &readingDone,

		formatter:     formatter,
		readerOptions: options,
	}

	go func() {
		defer func() {
			PanicHandler("newReaderFromStream()/readStream()", recover(), debug.Stack())
		}()

		returnMe.readStream(reader, formatter, options)
	}()

	return &returnMe
}

// Testing only!! May or may not hang if run in real world scenarios.
//
// NewFromTextForTesting creates a Reader from a block of text.
//
// First parameter is the name of this Reader. This name will be displayed by
// Moor in the bottom left corner of the screen.
//
// Calling Wait() on this Reader will always return immediately, no
// asynchronous ops will be performed.
func NewFromTextForTesting(name string, text string) *ReaderImpl {
	noExternalNewlines := strings.Trim(text, "\n")
	lines := []*Line{}
	if len(noExternalNewlines) > 0 {
		for _, lineString := range strings.Split(noExternalNewlines, "\n") {
			line := Line{raw: []byte(lineString)}
			lines = append(lines, &line)
		}
	}
	readingDone := atomic.Bool{}
	readingDone.Store(true)
	highlightingDone := atomic.Bool{}
	highlightingDone.Store(true) // No highlighting to do = nothing left = Done!
	returnMe := &ReaderImpl{
		lines:                   lines,
		ReadingDone:             &readingDone,
		HighlightingDone:        &highlightingDone,
		doneWaitingForFirstByte: make(chan bool, 1),
	}
	if name != "" {
		returnMe.DisplayName = &name
	}

	return returnMe
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

// NewFromFilename creates a new file reader.
//
// If options.Lexer is nil it will be determined from the input file name.
//
// If options.Style is nil, you must call reader.SetStyleForHighlighting() later
// to get highlighting.
//
// The Reader will try to uncompress various compressed file format, and also
// apply highlighting to the file using Chroma:
// https://github.com/alecthomas/chroma
func NewFromFilename(filename string, formatter chroma.Formatter, options ReaderOptions) (*ReaderImpl, error) {
	fileError := TryOpen(filename)
	if fileError != nil {
		return nil, fileError
	}

	stream, highlightingFilename, err := ZOpen(filename)
	if err != nil {
		return nil, err
	}

	if options.Lexer == nil {
		options.Lexer = lexers.Match(highlightingFilename)
	}

	returnMe := newReaderFromStream(stream, &highlightingFilename, formatter, options)

	if options.Lexer == nil {
		returnMe.HighlightingDone.Store(true)
	}

	if options.Style != nil {
		returnMe.SetStyleForHighlighting(*options.Style)
	}

	return returnMe, nil
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

func textAsString(reader *ReaderImpl, shouldFormat bool) string {
	reader.RLock()

	text := []byte{}
	for _, line := range reader.lines {
		text = append(text, line.raw...)
		text = append(text, '\n')
	}
	reader.RUnlock()

	var jsonData any
	err := json.Unmarshal(text, &jsonData)
	if err != nil {
		// Not JSON, return the text as-is
		return string(text)
	}

	if !shouldFormat {
		log.Info("Try the --reformat flag for automatic JSON reformatting")
		return string(text)
	}

	// Pretty print the JSON
	prettyJSON, err := json.MarshalIndent(jsonData, "", "  ")
	if err != nil {
		log.Debug("Failed to pretty print JSON: ", err)
		return string(text)
	}

	log.Debug("Got the --reformat flag, reformatted JSON input")
	return string(prettyJSON)
}

func isXml(text string) bool {
	err := xml.Unmarshal([]byte(text), new(any))
	return err == nil
}

// We expect this to be executed in a goroutine
func highlightFromMemory(reader *ReaderImpl, formatter chroma.Formatter, options ReaderOptions) {
	// Is the buffer small enough?
	var byteCount int64
	reader.RLock()
	for _, line := range reader.lines {
		byteCount += int64(len(line.raw))

		if byteCount > MAX_HIGHLIGHT_SIZE {
			log.Info("File too large for highlighting: ", byteCount)
			reader.RUnlock()
			return
		}
	}
	reader.RUnlock()

	text := textAsString(reader, options.ShouldFormat)

	if len(text) == 0 {
		log.Debug("Buffer is empty, not highlighting")
		return
	}

	if options.Lexer == nil && json.Valid([]byte(text)) {
		log.Info("Buffer is valid JSON, highlighting as JSON")
		options.Lexer = lexers.Get("json")
	} else if options.Lexer == nil && isXml(text) {
		log.Info("Buffer is valid XML, highlighting as XML")
		options.Lexer = lexers.Get("xml")
	}

	if options.Lexer == nil {
		log.Debug("No lexer set, not highlighting")
		return
	}

	if options.Style == nil {
		log.Debug("No style set, not highlighting")
		return
	}

	if formatter == nil {
		log.Debug("No formatter set, not highlighting")
		return
	}

	highlighted, err := Highlight(text, *options.Style, formatter, options.Lexer)
	if err != nil {
		log.Warn("Highlighting failed: ", err)
		return
	}

	if highlighted == nil {
		// No highlighting would be done, never mind
		return
	}

	reader.setText(*highlighted)
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

// The index is for error reporting. Set withCache to false to simulate a cache
// miss for benchmarking.
func (line *Line) Plain(index linemetadata.Index) string {
	fromCache := line.plainTextCache.Load()
	if DisablePlainCachingForBenchmarking {
		// Simulate a cache miss for benchmarking
		fromCache = nil
	}
	if fromCache != nil {
		return *fromCache
	}

	plain := textstyles.StripFormatting(string(line.raw), index)

	// If this succeeds, all good. If it fails it means some other goroutine
	// populated the cache before us, which is also fine.
	_ = line.plainTextCache.CompareAndSwap(nil, &plain)

	return plain
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
