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
	assert.Assert(t, !r.ReadingDone(), "Test setup broken, reading should not be done")

	assert.NilError(t, os.WriteFile(file, []byte(linesForTesting("new", lineCount)), 0o600))
	typeRunes(t, screen, pagingDone, "r")

	screen.waitForRow(t, "new0")
}

// Reloading must not flash an empty screen before the reloaded lines arrive.
// Ref: https://github.com/walles/moor/pull/478
func TestReloadShowsNoEmptyFrame(t *testing.T) {
	// Enough lines that the reloaded reader needs a moment before its first
	// lines arrive. It counts all lines before reading any of them.
	lineCount := 100_000
	file := filepath.Join(t.TempDir(), "lines.txt")
	assert.NilError(t, os.WriteFile(file, []byte(linesForTesting("old", lineCount)), 0o600))

	r, err := reader.NewFromFilename(file, formatters.TTY16m, reader.ReaderOptions{Style: colorlessChromaStyle})
	assert.NilError(t, err)

	pager := NewPager(r)
	pager.ShowLineNumbers = false
	screen := newPagingLoopScreen(20, 10)
	pagingDone := startPagingInBackground(t, pager, screen)
	screen.waitForRow(t, "old0")

	assert.NilError(t, os.WriteFile(file, []byte(linesForTesting("new", lineCount)), 0o600))
	framesBeforeReload := len(screen.shownFrames())
	typeRunes(t, screen, pagingDone, "r")
	screen.waitForRow(t, "new0")

	for _, frame := range screen.shownFrames()[framesBeforeReload:] {
		assert.Assert(t, frame[0] == "old0" || frame[0] == "new0",
			"Expected the first line at the top after reloading, got:\n%s", strings.Join(frame, "\n"))
	}
}

// Reload a file with this many lines, and check that we didn't wait for lines
// that will never come
func testReloadWithoutWaiting(t *testing.T, lineCount int) {
	file := filepath.Join(t.TempDir(), "lines.txt")
	assert.NilError(t, os.WriteFile(file, []byte(linesForTesting("old", lineCount)), 0o600))

	r, err := reader.NewFromFilename(file, formatters.TTY16m, reader.ReaderOptions{Style: colorlessChromaStyle})
	assert.NilError(t, err)
	assert.NilError(t, r.Wait())

	pager := NewPager(r)
	pager.ShowLineNumbers = false
	screen := newPagingLoopScreen(20, 10)
	pagingDone := startPagingInBackground(t, pager, screen)
	screen.waitForRow(t, "---") // End of input marker, we're up and running

	t0 := time.Now()
	typeRunes(t, screen, pagingDone, "r")
	// The main loop takes this only after it's done reloading and redrawing
	handEvent(t, screen, pagingDone, twin.EventResize{})
	elapsed := time.Since(t0)

	// If this turns out to be flaky, the machine stalled for longer than
	// reloadWaitLimit during a reload that normally takes a few milliseconds.
	// Waiting out the whole limit, which is the bug this test catches, always
	// takes longer than that.
	assert.Assert(t, elapsed < reloadWaitLimit,
		"Reloading took %s, looks like we waited for lines that never came", elapsed)
}

func TestReloadEmptyFileWithoutWaiting(t *testing.T) {
	testReloadWithoutWaiting(t, 0)
}

// Fewer lines than the screen wants
func TestReloadShortFileWithoutWaiting(t *testing.T) {
	testReloadWithoutWaiting(t, 3)
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

// Switching to a file that was rewritten on disk while we were away should take
// us straight back to where we were in it, without first showing some other
// part of it
func TestSwitchingToRewrittenFileShowsOnlyTarget(t *testing.T) {
	// A file we leave at its top
	a := reader.NewFromTextForTesting("a", linesForTesting("a", 3))

	file := filepath.Join(t.TempDir(), "lines.txt")
	assert.NilError(t, os.WriteFile(file, []byte(linesForTesting("old", 20_000)), 0o600))
	b, err := reader.NewFromFilename(file, formatters.TTY16m, reader.ReaderOptions{Style: colorlessChromaStyle})
	assert.NilError(t, err)

	pager := NewPager(b, a)
	pager.ShowLineNumbers = false
	pager.searchHistory = &SearchHistory{} // No file backing, keep this test disk-free
	screen := newPagingLoopScreen(20, 10)
	pagingDone := startPagingInBackground(t, pager, screen)
	screen.waitForRow(t, "old0")

	typeRunes(t, screen, pagingDone, "g15000")
	handEvent(t, screen, pagingDone, twin.EventKeyCode{KeyCode: twin.KeyEnter})
	screen.waitForTopRow(t, "old14999")
	waitForWatching(t, pager)

	typeRunes(t, screen, pagingDone, ":n")
	screen.waitForTopRow(t, "a0")

	// Shorter, so that this must be a reload rather than an append
	assert.NilError(t, os.WriteFile(file, []byte(linesForTesting("new", 19_000)), 0o600))
	waitUntil(t, b.WantsReload, func() string { return "the rewritten file to be noticed" })

	framesBeforeSwitch := len(screen.shownFrames())
	typeRunes(t, screen, pagingDone, ":p")
	screen.waitForTopRow(t, "new14999")

	for _, frame := range screen.shownFrames()[framesBeforeSwitch:] {
		assert.Assert(t, frame[0] == "a0" || frame[0] == "new14999",
			"Expected only the target at the top after switching, got:\n%s", strings.Join(frame, "\n"))
	}
}

// Open a file with old lines and go to the given line
func startPagingAtLine(t *testing.T, file string, lineNumber int) (*Pager, *pagingLoopScreen, chan struct{}) {
	t.Helper()

	r, err := reader.NewFromFilename(file, formatters.TTY16m, reader.ReaderOptions{Style: colorlessChromaStyle})
	assert.NilError(t, err)

	pager := NewPager(r)
	pager.ShowLineNumbers = false
	pager.searchHistory = &SearchHistory{} // No file backing, keep this test disk-free
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
	assert.Assert(t, !original.ReadingDone(), "Test setup broken, reading should not be done")

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

// A file rewritten on disk while we are in some other mode than viewing should
// take us back to where we were once we're back to viewing
func TestRewrittenFileInOtherModeKeepsPosition(t *testing.T) {
	for _, mode := range []struct {
		name  string
		enter string
		leave twin.Event
	}{
		{"filter", "&", twin.EventKeyCode{KeyCode: twin.KeyEnter}},
		{"go to line, Esc", "g", twin.EventKeyCode{KeyCode: twin.KeyEscape}},
		{"go to line, q", "g", twin.EventRune{Rune: 'q'}},
	} {
		t.Run(mode.name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "lines.txt")
			assert.NilError(t, os.WriteFile(file, []byte(linesForTesting("old", 100_000)), 0o600))
			pager, screen, pagingDone := startPagingAtLine(t, file, 90_000)
			waitForWatching(t, pager)

			pager.readerLock.Lock()
			original := pager.views[pager.currentView].reader
			pager.readerLock.Unlock()

			typeRunes(t, screen, pagingDone, mode.enter)

			// Shorter, so that this must be a reload rather than an append
			assert.NilError(t, os.WriteFile(file, []byte(linesForTesting("new", 95_000)), 0o600))
			var reloaded *reader.ReaderImpl
			waitUntil(t, func() bool {
				pager.readerLock.Lock()
				defer pager.readerLock.Unlock()
				reloaded = pager.views[pager.currentView].reader
				return reloaded != original
			}, func() string { return "the rewritten file to be reloaded" })
			waitUntil(t, reloaded.ReadingDone, func() string { return "the rewritten file to be read" })

			// The background goroutine throttles more-lines notifications for
			// 200ms. Wait for any last one to arrive while we're still in the
			// mode. Arriving after we left, it would take us to the right
			// place, and this test would pass whether or not leaving the mode
			// does.
			time.Sleep(300 * time.Millisecond)

			handEvent(t, screen, pagingDone, mode.leave)
			screen.waitForTopRow(t, "new89999")
		})
	}
}

// Like linesForTesting(), but every 100th line says "hit"
func linesWithHits(prefix string, count int) string {
	var lines strings.Builder
	for i := range count {
		if i%100 == 50 {
			fmt.Fprintf(&lines, "%shit%d\n", prefix, i)
		} else {
			fmt.Fprintf(&lines, "%s%d\n", prefix, i)
		}
	}
	return lines.String()
}

// Reloading with a filter active should take us back to where we were in the
// filtered lines, even if the filter matches only a few lines
func TestRewrittenFileKeepsFilteredPosition(t *testing.T) {
	file := filepath.Join(t.TempDir(), "lines.txt")
	assert.NilError(t, os.WriteFile(file, []byte(linesWithHits("old", 100_000)), 0o600))

	// Far enough down that we read all lines, so that we watch the file for
	// changes
	pager, screen, pagingDone := startPagingAtLine(t, file, 90_000)
	waitForWatching(t, pager)

	typeRunes(t, screen, pagingDone, "&hit")
	handEvent(t, screen, pagingDone, twin.EventKeyCode{KeyCode: twin.KeyEnter})
	screen.waitForRow(t, "oldhit99950")

	// Filtered line 800 is unfiltered line 79951. Reading 50k unfiltered lines
	// gets us only 500 filtered ones.
	typeRunes(t, screen, pagingDone, "g800")
	handEvent(t, screen, pagingDone, twin.EventKeyCode{KeyCode: twin.KeyEnter})
	screen.waitForTopRow(t, "oldhit79950")

	// Shorter, so that this must be a reload rather than an append
	assert.NilError(t, os.WriteFile(file, []byte(linesWithHits("new", 95_000)), 0o600))

	screen.waitForTopRow(t, "newhit79950")
}

// Scrolling up with the mouse wheel should stop following, just like scrolling
// up with the keyboard does
func TestMouseWheelStopsFollowing(t *testing.T) {
	file := filepath.Join(t.TempDir(), "lines.txt")
	assert.NilError(t, os.WriteFile(file, []byte(linesForTesting("old", 100)), 0o600))

	r, err := reader.NewFromFilename(file, formatters.TTY16m, reader.ReaderOptions{Style: colorlessChromaStyle})
	assert.NilError(t, err)
	assert.NilError(t, r.Wait())

	pager := NewPager(r)
	pager.ShowLineNumbers = false
	screen := newPagingLoopScreen(40, 10) // Wide enough for the line count
	pagingDone := startPagingInBackground(t, pager, screen)
	screen.waitForRow(t, "old0")

	typeRunes(t, screen, pagingDone, "G")
	screen.waitForTopRow(t, "old91")

	handEvent(t, screen, pagingDone, twin.EventMouse{Buttons: twin.MouseWheelUp})
	screen.waitForTopRow(t, "old90")

	appendTo, err := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0)
	assert.NilError(t, err)
	_, err = appendTo.WriteString(linesForTesting("new", 100))
	assert.NilError(t, err)
	assert.NilError(t, appendTo.Close())

	// Best effort: this frame is usually drawn after handling the new lines,
	// which is when following would have moved us. A redraw for some other
	// reason, like the spinner, could show the new line count earlier though,
	// and then this test would pass even with following still on.
	var rows []string
	waitUntil(t, func() bool {
		rows = screen.shownRows()
		return slices.ContainsFunc(rows, func(row string) bool {
			return strings.Contains(row, "200 lines")
		})
	}, func() string {
		return fmt.Sprintf("the appended lines to be counted, last frame:\n%s", strings.Join(rows, "\n"))
	})

	assert.Equal(t, rows[0], "old90", "Last frame:\n%s", strings.Join(rows, "\n"))
}

// Scrolling down to the end with the mouse wheel should start following, just
// like scrolling down with the keyboard does
func TestMouseWheelStartsFollowing(t *testing.T) {
	file := filepath.Join(t.TempDir(), "lines.txt")
	assert.NilError(t, os.WriteFile(file, []byte(linesForTesting("old", 100)), 0o600))

	r, err := reader.NewFromFilename(file, formatters.TTY16m, reader.ReaderOptions{Style: colorlessChromaStyle})
	assert.NilError(t, err)
	assert.NilError(t, r.Wait())

	pager := NewPager(r)
	pager.ShowLineNumbers = false
	screen := newPagingLoopScreen(40, 10)
	pagingDone := startPagingInBackground(t, pager, screen)
	screen.waitForRow(t, "old0")

	// 100 lines on a screen with room for 9 means old91 at the top at the end
	for range 91 {
		handEvent(t, screen, pagingDone, twin.EventMouse{Buttons: twin.MouseWheelDown})
	}
	screen.waitForTopRow(t, "old91")

	appendTo, err := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0)
	assert.NilError(t, err)
	_, err = appendTo.WriteString(linesForTesting("new", 100))
	assert.NilError(t, err)
	assert.NilError(t, appendTo.Close())

	screen.waitForTopRow(t, "new91")
}

// Searching reads the whole file, so that hits far down can be found. Scrolling
// with the mouse wheel while typing the search term shouldn't stop that.
func TestMouseWheelWhileSearchingKeepsReading(t *testing.T) {
	file := filepath.Join(t.TempDir(), "lines.txt")
	lines := linesForTesting("line", 20*reader.DEFAULT_PAUSE_AFTER_LINES) + "needle\n"
	assert.NilError(t, os.WriteFile(file, []byte(lines), 0o600))

	r, err := reader.NewFromFilename(file, formatters.TTY16m, reader.ReaderOptions{Style: colorlessChromaStyle})
	assert.NilError(t, err)

	pager := NewPager(r)
	pager.ShowLineNumbers = false
	pager.searchHistory = &SearchHistory{} // No file backing, keep this test disk-free
	screen := newPagingLoopScreen(40, 10)
	pagingDone := startPagingInBackground(t, pager, screen)
	screen.waitForRow(t, "line0")

	typeRunes(t, screen, pagingDone, "/")
	handEvent(t, screen, pagingDone, twin.EventMouse{Buttons: twin.MouseWheelDown})
	handEvent(t, screen, pagingDone, twin.EventMouse{Buttons: twin.MouseWheelUp})
	typeRunes(t, screen, pagingDone, "needle")

	screen.waitForRow(t, "needle")
}
