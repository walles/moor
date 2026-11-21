package internal

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/walles/twin"
	"gotest.tools/v3/assert"

	"github.com/walles/moor/v2/internal/reader"
)

// Following must keep working in an input we switched to
func TestFollowingWorksAfterSwitchingFiles(t *testing.T) {
	a := reader.NewFromTextForTesting("a", linesForTesting("a", 300))
	assert.NilError(t, a.Wait())

	bReader, bWriter := io.Pipe()
	t.Cleanup(func() { _ = bWriter.Close() })
	go func() {
		// The constructor peeks at the stream, so give it something to see
		_, _ = io.WriteString(bWriter, "b0\n")
	}()
	b, err := reader.NewFromStream("b", bReader, formatters.TTY16m, reader.ReaderOptions{Style: colorlessChromaStyle})
	assert.NilError(t, err)

	pager := NewPager(a, b)
	pager.ShowLineNumbers = false
	screen := newPagingLoopScreen(20, 10)
	pagingDone := startPagingInBackground(t, pager, screen)
	screen.waitForRow(t, "a0")

	// The background goroutine throttles more-lines notifications for 200ms,
	// and picks up the current input when that throttling ends. Switch after
	// that, or this test passes even if switching never tells the goroutine.
	time.Sleep(300 * time.Millisecond)

	typeRunes(t, screen, pagingDone, ":n")
	screen.waitForRow(t, "b0")
	typeRunes(t, screen, pagingDone, "G") // Start following

	// More than a screenful, so we only see the last line if we follow
	for i := 1; i <= 30; i++ {
		_, err := fmt.Fprintf(bWriter, "b%d\n", i)
		assert.NilError(t, err)
	}

	screen.waitForRow(t, "b30")
}

func TestReloadKeepsFilter(t *testing.T) {
	file := filepath.Join(t.TempDir(), "lines.txt")
	assert.NilError(t, os.WriteFile(file, []byte("apa\nbepa\ncepa\n"), 0o600))

	r, err := reader.NewFromFilename(file, formatters.TTY16m, reader.ReaderOptions{Style: colorlessChromaStyle})
	assert.NilError(t, err)
	assert.NilError(t, r.Wait())

	pager := NewPager(r)
	pager.ShowLineNumbers = false
	screen := newPagingLoopScreen(20, 10)
	pagingDone := startPagingInBackground(t, pager, screen)

	typeRunes(t, screen, pagingDone, "&bepa")
	handEvent(t, screen, pagingDone, twin.EventKeyCode{KeyCode: twin.KeyEnter})
	rows := screen.waitForRow(t, "bepa")
	assert.Equal(t, rows[0], "bepa")

	// Change the file so we can tell when the reload shows up
	assert.NilError(t, os.WriteFile(file, []byte("apa\nbepa2\ncepa\n"), 0o600))
	typeRunes(t, screen, pagingDone, "r")

	rows = screen.waitForRow(t, "bepa2")
	assert.Equal(t, rows[0], "bepa2", "Filter should survive reload")
}

// Pressing 'r' must reload even if we haven't finished reading the file yet
func TestReloadWhileStillReading(t *testing.T) {
	// More lines than the pager will have us read without scrolling, so that
	// reading stays unfinished
	lineCount := 2 * reader.DEFAULT_PAUSE_AFTER_LINES
	file := filepath.Join(t.TempDir(), "lines.txt")
	assert.NilError(t, os.WriteFile(file, []byte(linesForTesting("old", lineCount)), 0o600))

	r, err := reader.NewFromFilename(file, formatters.TTY16m, reader.ReaderOptions{Style: colorlessChromaStyle})
	assert.NilError(t, err)

	pager := NewPager(r)
	pager.ShowLineNumbers = false
	screen := newPagingLoopScreen(20, 10)
	pagingDone := startPagingInBackground(t, pager, screen)
	screen.waitForRow(t, "old0")
	assert.Assert(t, !r.ReadingDone.Load(), "Test setup broken, reading should not be done")

	assert.NilError(t, os.WriteFile(file, []byte(linesForTesting("new", lineCount)), 0o600))
	typeRunes(t, screen, pagingDone, "r")

	screen.waitForRow(t, "new0")
}

// A file rewritten on disk must show up rewritten in the pager
func TestRewrittenFileIsReloaded(t *testing.T) {
	file := filepath.Join(t.TempDir(), "lines.txt")
	assert.NilError(t, os.WriteFile(file, []byte(linesForTesting("old", 3)), 0o600))

	r, err := reader.NewFromFilename(file, formatters.TTY16m, reader.ReaderOptions{Style: colorlessChromaStyle})
	assert.NilError(t, err)

	pager := NewPager(r)
	pager.ShowLineNumbers = false
	screen := newPagingLoopScreen(20, 10)
	startPagingInBackground(t, pager, screen)
	screen.waitForRow(t, "old0")

	// Shorter, so that this must be a reload rather than an append
	assert.NilError(t, os.WriteFile(file, []byte(linesForTesting("new", 2)), 0o600))

	screen.waitForRow(t, "new0")
}

// A file rewritten on disk while we were looking at another one must show up
// rewritten when we switch to it
func TestRewrittenFileIsReloadedWhenSwitchedTo(t *testing.T) {
	a := reader.NewFromTextForTesting("a", linesForTesting("a", 3))

	file := filepath.Join(t.TempDir(), "lines.txt")
	assert.NilError(t, os.WriteFile(file, []byte(linesForTesting("old", 3)), 0o600))
	b, err := reader.NewFromFilename(file, formatters.TTY16m, reader.ReaderOptions{Style: colorlessChromaStyle})
	assert.NilError(t, err)
	assert.NilError(t, b.Wait())

	pager := NewPager(a, b)
	pager.ShowLineNumbers = false
	screen := newPagingLoopScreen(20, 10)
	pagingDone := startPagingInBackground(t, pager, screen)
	screen.waitForRow(t, "a0")

	// Shorter, so that this must be a reload rather than an append
	assert.NilError(t, os.WriteFile(file, []byte(linesForTesting("new", 2)), 0o600))
	waitUntil(t, b.WantsReload, func() string { return "the rewritten file to be noticed" })

	framesBeforeSwitch := len(screen.shownFrames())
	typeRunes(t, screen, pagingDone, ":n")
	screen.waitForRow(t, "new0")

	for _, frame := range screen.shownFrames()[framesBeforeSwitch:] {
		assert.Assert(t, !slices.Contains(frame, "old0"),
			"Stale content shown after switching:\n%s", strings.Join(frame, "\n"))
	}
}

// Open a file with old lines and go to the given line
func startPagingAtLine(t *testing.T, file string, lineNumber int) (*Pager, *pagingLoopScreen, chan struct{}) {
	t.Helper()

	r, err := reader.NewFromFilename(file, formatters.TTY16m, reader.ReaderOptions{Style: colorlessChromaStyle})
	assert.NilError(t, err)

	pager := NewPager(r)
	pager.ShowLineNumbers = false
	screen := newPagingLoopScreen(20, 10)
	pagingDone := startPagingInBackground(t, pager, screen)
	screen.waitForRow(t, "old0")

	typeRunes(t, screen, pagingDone, fmt.Sprintf("g%d", lineNumber))
	handEvent(t, screen, pagingDone, twin.EventKeyCode{KeyCode: twin.KeyEnter})
	screen.waitForRow(t, fmt.Sprintf("old%d", lineNumber-1))

	return pager, screen, pagingDone
}

// Wait until the top row says exactly this
func (screen *pagingLoopScreen) waitForTopRow(t *testing.T, want string) {
	t.Helper()

	var rows []string
	waitUntil(t, func() bool {
		rows = screen.shownRows()
		return len(rows) > 0 && rows[0] == want
	}, func() string {
		return fmt.Sprintf("%q at the top, last frame:\n%s", want, strings.Join(rows, "\n"))
	})
}

// Readers watch their files for changes only after reading all of them
func waitForWatching(t *testing.T, pager *Pager) {
	t.Helper()

	pager.readerLock.Lock()
	r := pager.views[pager.currentView].reader
	pager.readerLock.Unlock()

	waitUntil(t, r.HighlightingDone.Load, func() string { return "the file to be read to the end" })
}

// After the file is rewritten on disk, we should be where we were before
func TestRewrittenFileKeepsPosition(t *testing.T) {
	file := filepath.Join(t.TempDir(), "lines.txt")
	assert.NilError(t, os.WriteFile(file, []byte(linesForTesting("old", 100_000)), 0o600))
	pager, screen, _ := startPagingAtLine(t, file, 90_000)
	waitForWatching(t, pager)

	// Shorter, so that this must be a reload rather than an append
	assert.NilError(t, os.WriteFile(file, []byte(linesForTesting("new", 95_000)), 0o600))

	screen.waitForTopRow(t, "new89999")
}

// After pressing 'r', we should be where we were before
func TestReloadKeepsPosition(t *testing.T) {
	// So many lines that we won't read to the end, and thus won't watch the
	// file for changes. That leaves 'r' as the only way to reload it.
	file := filepath.Join(t.TempDir(), "lines.txt")
	assert.NilError(t, os.WriteFile(file, []byte(linesForTesting("old", 200_000)), 0o600))
	pager, screen, pagingDone := startPagingAtLine(t, file, 90_000)

	pager.readerLock.Lock()
	original := pager.views[pager.currentView].reader
	pager.readerLock.Unlock()
	assert.Assert(t, !original.ReadingDone.Load(), "Test setup broken, reading should not be done")

	assert.NilError(t, os.WriteFile(file, []byte(linesForTesting("new", 200_000)), 0o600))
	typeRunes(t, screen, pagingDone, "r")

	screen.waitForTopRow(t, "new89999")
}

// A reload while the help screen is up should take us back to where we were
// when we leave the help screen
func TestReloadDuringHelpKeepsPosition(t *testing.T) {
	file := filepath.Join(t.TempDir(), "lines.txt")
	assert.NilError(t, os.WriteFile(file, []byte(linesForTesting("old", 100_000)), 0o600))
	pager, screen, pagingDone := startPagingAtLine(t, file, 90_000)
	waitForWatching(t, pager)

	pager.readerLock.Lock()
	original := pager.views[pager.currentView].reader
	pager.readerLock.Unlock()

	typeRunes(t, screen, pagingDone, "h")
	screen.waitForRow(t, "Miscellaneous")

	// Shorter, so that this must be a reload rather than an append
	assert.NilError(t, os.WriteFile(file, []byte(linesForTesting("new", 95_000)), 0o600))
	waitUntil(t, func() bool {
		pager.readerLock.Lock()
		defer pager.readerLock.Unlock()
		return pager.views[pager.currentView].reader != original
	}, func() string { return "the rewritten file to be reloaded" })

	typeRunes(t, screen, pagingDone, "q") // Leave help
	screen.waitForTopRow(t, "new89999")
}
