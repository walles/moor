package reader

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/klauspost/compress/zstd"
	"gotest.tools/v3/assert"
)

func TestTryOpenDirectory(t *testing.T) {
	tempDir := t.TempDir()

	err := TryOpen(tempDir)
	assert.Assert(t, err != nil, "TryOpen should fail on directories")
}

func TestReadTextDone(t *testing.T) {
	testMe := NewFromTextForTesting("", "Johan")

	assert.NilError(t, testMe.Wait())
}

type panickingFormatter struct{}

func (formatter panickingFormatter) Format(io.Writer, *chroma.Style, chroma.Iterator) error {
	panic("Highlighting blew up")
}

// A reader that panics on the way is still done afterwards, so that whoever
// waits for it can stop waiting.
//
// Panics in a reader are logged rather than propagated, so without this the
// pager would hold back its first paint forever waiting for highlighting, and
// spin its spinner forever waiting for reading.
func TestReaderIsDoneAfterHighlightingPanics(t *testing.T) {
	testMe, err := NewFromStream("", strings.NewReader(`{"one": 1}`),
		panickingFormatter{}, ReaderOptions{Lexer: lexers.Get("json")})
	assert.NilError(t, err)

	// Highlighting waits for this, and panics once it arrives
	testMe.SetStyleForHighlighting(*styles.Get("native"))

	giveUp := time.After(time.Second)
	for !testMe.HighlightingDone.Load() {
		select {
		case <-testMe.MaybeDone:
			// Progress, look again

		case <-giveUp:
			t.Fatal("Highlighting never reported done after panicking")
		}
	}

	assert.Assert(t, testMe.ReadingDone(), "Reading should be done as well")
}

// NewFromStream takes ownership of the stream, so once it has consumed the
// initial contents it closes it. Reading from the pipe afterwards should
// therefore report it closed rather than just empty.
//
// Compressed input gets handed to a decompressor rather than read directly, one
// decompressor per format, so ownership has to survive each of those.
func TestStreamClosedAfterReading(t *testing.T) {
	streams := map[string][]byte{
		"uncompressed": []byte("Johan\n"),
		"empty":        nil,
	}
	for _, sampleName := range []string{
		"compressed.txt.gz",
		"compressed.txt.bz2",
		"compressed.txt.xz",
		"compressed.txt.zst",
	} {
		fileContents, err := os.ReadFile(path.Join(samplesDir, sampleName))
		assert.NilError(t, err)
		streams[sampleName] = fileContents
	}

	for name, streamContents := range streams {
		t.Run(name, func(t *testing.T) {
			pipeReader, pipeWriter := io.Pipe()

			go func() {
				if len(streamContents) == 0 {
					// Writing nothing would still show up as a zero length read,
					// and we want an immediate end of stream
					pipeWriter.Close() //nolint:errcheck
					return
				}

				_, err := pipeWriter.Write(streamContents)

				// With nil this is just a Close(), signalling end of stream
				pipeWriter.CloseWithError(err) //nolint:errcheck
			}()

			testMe, err := NewFromStream("", pipeReader, nil, ReaderOptions{Style: &chroma.Style{}})
			assert.NilError(t, err)
			assert.NilError(t, testMe.Wait())

			_, err = pipeReader.Read(make([]byte, 1))
			assert.Equal(t, err, io.ErrClosedPipe)
		})
	}
}

// Hands out the first `fed` bytes of `data` immediately, then blocks until
// told to proceed, signalling on `started` right before it blocks. This lets
// a test know for certain that whoever is reading from it is still inside
// that Read() call, rather than guessing from timing.
type blockingReader struct {
	data    []byte
	fed     int
	pos     int
	started chan struct{}
	proceed chan struct{}
	once    sync.Once
}

func (r *blockingReader) Read(p []byte) (int, error) {
	if r.pos < r.fed {
		n := copy(p, r.data[r.pos:r.fed])
		r.pos += n
		return n, nil
	}

	r.once.Do(func() { close(r.started) })
	<-r.proceed
	return 0, io.ErrClosedPipe
}

// Closing a reader whose zstd stream is still being decompressed must unblock
// that decompression rather than racing with it, so this has to complete
// rather than hang.
func TestZstdReaderClosedWhileReading(t *testing.T) {
	text := bytes.Repeat([]byte("hello world, this is a line of text\n"), 200_000)

	var compressed bytes.Buffer
	encoder, err := zstd.NewWriter(&compressed)
	assert.NilError(t, err)
	_, err = encoder.Write(text)
	assert.NilError(t, err)
	assert.NilError(t, encoder.Close())

	src := &blockingReader{
		data:    compressed.Bytes(),
		fed:     len(compressed.Bytes()) / 2,
		started: make(chan struct{}),
		proceed: make(chan struct{}),
	}

	// Disable the default pause-after-N-lines behavior: this highly
	// compressible text decompresses to far more than that limit's worth of
	// lines using only the first half of the compressed bytes, and we need
	// reading to keep going until it genuinely runs out of input.
	noPause := math.MaxInt
	testMe, err := NewFromStream("", src, nil, ReaderOptions{Style: &chroma.Style{}, PauseAfterLines: &noPause})
	assert.NilError(t, err)

	select {
	case <-src.started:
	case <-time.After(5 * time.Second):
		t.Fatal("never saw a blocked Read(), test setup is broken")
	}

	testMe.Close()
	close(src.proceed)

	// Only finishing matters here, not what Wait() returns after Close()
	done := make(chan struct{})
	go func() {
		_ = testMe.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Close() while still decompressing should not hang")
	}
}

// Closing the reader closes the stream, which is what tells a producer still
// writing into it that nobody is listening any more.
func TestStreamClosedOnReaderClose(t *testing.T) {
	pipeReader, pipeWriter := io.Pipe()

	// The pipe is unbuffered and NewFromStream reads from the stream before
	// returning, so this write has to happen concurrently with that call.
	firstWrite := make(chan error, 1)
	go func() {
		_, err := pipeWriter.Write([]byte("Johan\n"))
		firstWrite <- err
	}()

	testMe, err := NewFromStream("", pipeReader, nil, ReaderOptions{Style: &chroma.Style{}})
	assert.NilError(t, err)

	// Once this write has landed the reader is waiting for more, with the
	// stream still open.
	assert.NilError(t, <-firstWrite)

	testMe.Close()

	secondWrite := make(chan error, 1)
	go func() {
		_, err := pipeWriter.Write([]byte("Nobody home\n"))
		secondWrite <- err
	}()

	select {
	case err := <-secondWrite:
		assert.Equal(t, err, io.ErrClosedPipe)
	case <-time.After(time.Second):
		t.Fatal("Writing to a closed reader's stream should fail, it blocked instead")
	}
}

// Writes a Go source file with lineCount lines and returns its name
func writeGoSource(t *testing.T, lineCount int) string {
	t.Helper()

	var source strings.Builder
	source.WriteString("package main\n")
	for i := range lineCount - 1 {
		fmt.Fprintf(&source, "var x%d = %d\n", i, i)
	}

	fileName := path.Join(t.TempDir(), "source.go")
	assert.NilError(t, os.WriteFile(fileName, []byte(source.String()), 0o600))
	return fileName
}

func assertHighlighted(t *testing.T, testMe *ReaderImpl) {
	t.Helper()

	testMe.RLock()
	defer testMe.RUnlock()
	assert.Assert(t, len(testMe.lines) > 0)
	assert.Assert(t, bytes.Contains(testMe.lines[0].raw, []byte("\x1b[")),
		"Expected highlighting, got: %q", testMe.lines[0].raw)
}

// A clone made after the original has been highlighted is highlighted as well.
func TestCloneOfHighlightedReaderIsHighlighted(t *testing.T) {
	original, err := NewFromFilename(writeGoSource(t, 10), formatters.TTY16m, ReaderOptions{})
	assert.NilError(t, err)
	defer original.Close()
	original.SetStyleForHighlighting(*styles.Get("native"))
	assert.NilError(t, original.Wait())
	assertHighlighted(t, original)

	clone, err := original.Clone()
	assert.NilError(t, err)
	defer clone.Close()

	assert.NilError(t, clone.Wait())
	assertHighlighted(t, clone)
}

// A clone made while the original is still reading is highlighted once it is
// done reading. The original gets its style before it is done reading, but only
// looks at it afterwards, so the style must not get lost when cloning in
// between.
//
// If this test hangs, the clone is waiting for a style nobody is going to send
// it.
func TestCloneOfReaderStillReadingIsHighlighted(t *testing.T) {
	// Pausing makes sure the original stays unfinished, at least until we call
	// Wait() on it, which we don't
	pauseAfterLines := 2
	original, err := NewFromFilename(writeGoSource(t, 10), formatters.TTY16m, ReaderOptions{
		PauseAfterLines: &pauseAfterLines,
	})
	assert.NilError(t, err)
	defer original.Close()
	original.SetStyleForHighlighting(*styles.Get("native"))

	clone, err := original.Clone()
	assert.NilError(t, err)
	defer clone.Close()

	// The clone inherits the pause limit. Wait() bumps it whenever the clone
	// pauses.
	assert.NilError(t, clone.Wait())
	assertHighlighted(t, clone)

	assert.Assert(t, !original.ReadingDone(), "Test setup broken, original should still be reading")
}

// A clone shows the same name as the original, even if the name was changed
// after the original was created
func TestCloneKeepsDisplayName(t *testing.T) {
	original, err := NewFromFilename(writeGoSource(t, 10), formatters.TTY16m, ReaderOptions{Style: styles.Get("native")})
	assert.NilError(t, err)
	defer original.Close()

	title := "My Title"
	original.Lock()
	original.DisplayName = &title
	original.Unlock()

	clone, err := original.Clone()
	assert.NilError(t, err)
	defer clone.Close()

	clone.RLock()
	defer clone.RUnlock()
	assert.Equal(t, *clone.DisplayName, title)
}

// A clone reads as far as the original was told to, not just as far as readers
// do by default
func TestCloneKeepsPauseLimit(t *testing.T) {
	original, err := NewFromFilename(writeGoSource(t, 10), formatters.TTY16m, ReaderOptions{Style: styles.Get("native")})
	assert.NilError(t, err)
	defer original.Close()

	pauseLimit := 3 * DEFAULT_PAUSE_AFTER_LINES
	original.SetPauseAfterLines(pauseLimit)

	clone, err := original.Clone()
	assert.NilError(t, err)
	defer clone.Close()

	clone.RLock()
	defer clone.RUnlock()
	assert.Equal(t, clone.pauseAfterLines, pauseLimit)
}

// Highlighting is wasted on a reader nobody is going to look at any more
func TestClosedReaderSkipsHighlighting(t *testing.T) {
	// Pausing keeps the reader from finishing before we close it
	pauseAfterLines := 2
	testMe, err := NewFromFilename(writeGoSource(t, 10), formatters.TTY16m, ReaderOptions{
		PauseAfterLines: &pauseAfterLines,
		Style:           styles.Get("native"),
	})
	assert.NilError(t, err)
	for !testMe.PauseStatus.Load() {
		time.Sleep(10 * time.Millisecond)
	}

	testMe.Close()
	for !testMe.HighlightingDone.Load() {
		time.Sleep(10 * time.Millisecond)
	}

	testMe.RLock()
	defer testMe.RUnlock()
	assert.Assert(t, len(testMe.lines) > 0)
	assert.Assert(t, !bytes.Contains(testMe.lines[0].raw, []byte("\x1b[")),
		"Expected no highlighting, got: %q", testMe.lines[0].raw)
}
