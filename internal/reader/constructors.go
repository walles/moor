package reader

// This file contains factory functions for creating Readers from various inputs.

import (
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync/atomic"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"

	"github.com/walles/moor/v2/internal/ptr"
)

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

	var initialStat os.FileInfo
	if fileStats, statErr := os.Stat(filename); statErr == nil {
		initialStat = fileStats
	}

	if options.Lexer == nil {
		options.Lexer = lexers.Match(highlightingFilename)
	}

	returnMe := newReaderFromStream(stream, &filename, formatter, options)
	returnMe.lastStat = initialStat

	// Ensure the display name matches the highlighting name (e.g. without .gz)
	returnMe.DisplayName = ptr.To(filepath.Base(highlightingFilename))

	returnMe.IsCompressed = (filename != highlightingFilename)

	if options.Lexer == nil {
		returnMe.HighlightingDone.Store(true)
	}

	if options.Style != nil {
		returnMe.SetStyleForHighlighting(*options.Style)
	}

	return returnMe, nil
}

// NewFromStream creates a new stream reader
//
// The reader takes ownership of the stream: if it implements io.Closer it will
// be closed once we have consumed its initial content, or on reader.Close(),
// whichever comes first.
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
// The reader takes ownership of the stream: if it implements io.Closer it will
// be closed once we have consumed its initial content, or on reader.Close(),
// whichever comes first.
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
		displayFileName = ptr.To(filepath.Base(*originalFileName))
	}
	streamCloser, _ := reader.(io.Closer)
	returnMe := ReaderImpl{
		FileName:    originalFileName,
		DisplayName: displayFileName,

		streamCloser: streamCloser,

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
			panicResult := recover()
			PanicHandler("newReaderFromStream()/readStream()", panicResult, debug.Stack())
			if panicResult == nil {
				return
			}

			// Whatever we got before the panic is all there is ever going to be,
			// so say we're done. Otherwise everybody waiting for us waits
			// forever: the pager holds its first paint until highlighting is
			// done, and it spins its spinner until reading is.
			returnMe.ReadingDone.Store(true)
			returnMe.HighlightingDone.Store(true)
			select {
			case returnMe.MaybeDone <- true:
			default:
			}
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
		for lineString := range strings.SplitSeq(noExternalNewlines, "\n") {
			lines = append(lines, ptr.To(Line{raw: []byte(lineString)}))
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

// Clone creates a new ReaderImpl using the same source file and options.
func (reader *ReaderImpl) Clone() (*ReaderImpl, error) {
	if reader.FileName == nil {
		return nil, nil // Ignore streams
	}

	reader.RLock()
	formatter := reader.formatter
	options := reader.readerOptions
	reader.RUnlock()

	return NewFromFilename(*reader.FileName, formatter, options)
}
