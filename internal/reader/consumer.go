package reader

// This file contains the logic for reading bytes from streams and splitting
// them into lines.

import (
	"bytes"
	"fmt"
	"io"
	"time"

	"github.com/alecthomas/chroma/v2"
	log "github.com/sirupsen/logrus"
)

// This is the reader's main function. It will be run in a goroutine. First it
// reads the stream until the end, then starts tailing.
func (reader *ReaderImpl) readStream(stream io.Reader, formatter chroma.Formatter, options ReaderOptions) {
	reader.consumeLinesFromStream(stream)

	// Close the initial stream as soon as we're done reading it, well before we
	// start tailing or doing expensive highlighting.
	reader.closeStream()

	reader.readingDone.Store(true)
	select {
	case reader.MaybeDone <- true:
	default:
	}

	if reader.closed.Load() {
		// Nobody will look at our lines any more, so don't highlight them, and
		// don't wait for a highlighting style that may never come
		reader.HighlightingDone.Store(true)
		select {
		case reader.MaybeDone <- true:
		default:
		}
		return
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

	// Special case, extend the previous line. Performance sensitive, validate
	// changes with BenchmarkReadLongLine().
	// Ref: https://github.com/walles/moor/issues/358
	lastIndex := len(reader.lines) - 1
	baseLine := reader.lines[lastIndex]

	// Publish a longer replacement rather than growing baseLine in place.
	// Renderers keep using *Line values after dropping our lock, so a published
	// Line's raw field must never be written again. See Line.rawString().
	//
	// Not from linePool: pooled Lines share one backing array, so a superseded
	// pool entry would keep its (now stale) raw bytes from being collected.
	reader.lines[lastIndex] = &Line{raw: append(baseLine.raw, line...)}

	return 0
}

// How much we read from the stream at a time. Lines longer than this arrive in
// several pieces.
//
// This value affects BenchmarkReadLargeFile() performance. Validate changes
// like this:
//
//	go test -benchmem -run='^$' -bench 'BenchmarkReadLargeFile' ./internal/reader
const byteBufferSize = 16 * 1024

// This function will update the Reader struct. It is expected to run in a
// goroutine.
//
// It is used both during the initial read of the stream until it ends, and
// while tailing files for changes.
func (reader *ReaderImpl) consumeLinesFromStream(stream io.Reader) {
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

	awaitingFirstByte := true
	for !reader.closed.Load() {
		byteBuffer := make([]byte, byteBufferSize)
		readBytes, err := stream.Read(byteBuffer)

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

		// Does this chunk start out continuing the last line?
		continuesLastLine := !reader.readSoFar.EndedWithNewline()

		lineStart := 0
		byteIndex := 0
		for readBytes > 0 {
			relativeNewlineLocation := bytes.IndexByte(byteBuffer[byteIndex:readBytes], '\n')
			if relativeNewlineLocation == -1 {
				// No more newlines in this buffer
				break
			}

			byteIndex += relativeNewlineLocation

			considerAppending := lineStart == 0 && continuesLastLine
			pauseDuration := reader.assumeLockAndAddLine(byteBuffer[lineStart:byteIndex], considerAppending, &linePool)
			t0 = t0.Add(pauseDuration)

			lineStart = byteIndex + 1
			byteIndex = lineStart
		}

		// Handle any remaining bytes as a partial line
		if lineStart < readBytes {
			considerAppending := lineStart == 0 && continuesLastLine
			pauseDuration := reader.assumeLockAndAddLine(byteBuffer[lineStart:readBytes], considerAppending, &linePool)
			t0 = t0.Add(pauseDuration)
		}

		// Lines added, track some stats!
		reader.readSoFar.Add(byteBuffer[:readBytes])

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
