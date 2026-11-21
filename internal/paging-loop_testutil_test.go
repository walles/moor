package internal

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/walles/twin"
)

// The wait helpers here have no deadline, "go test -timeout" ends stuck tests.
// Until then, they report what they are waiting for this often.
const stillWaitingInterval = 5 * time.Second

// Print to stderr rather than using t.Logf(). Without -v, the test log of a test
// that is still running is lost when "go test -timeout" kills it.
func reportStillWaiting(t *testing.T, format string, args ...any) {
	t.Helper()
	fmt.Fprintf(os.Stderr, "%s: still waiting for %s\n", t.Name(), fmt.Sprintf(format, args...))
}

// A screen that can run the pager main loop, and that counts paints and records
// what was painted.
//
// twin.FakeScreen.Events() returns nil, and receiving from a nil channel blocks
// forever, so the main loop cannot be run with a plain FakeScreen.
//
// Show() is what takes over the user's terminal, alternate screen included, so
// counting Show() calls is counting how much the user gets to see.
type pagingLoopScreen struct {
	*twin.FakeScreen
	events chan twin.Event
	shows  atomic.Int64

	frameLock sync.Mutex
	frames    [][]string // Rows of every shown frame, oldest first
}

func newPagingLoopScreen(width int, height int) *pagingLoopScreen {
	return &pagingLoopScreen{
		FakeScreen: twin.NewFakeScreen(width, height),

		// Unbuffered, so that sending on it is a handshake with the main loop
		events: make(chan twin.Event),
	}
}

func (screen *pagingLoopScreen) Events() chan twin.Event {
	return screen.events
}

// Called from the pager's main loop, which is the only one allowed to touch
// the FakeScreen cells. So we copy them here for the test to look at.
func (screen *pagingLoopScreen) Show() {
	screen.shows.Add(1)
	screen.FakeScreen.Show()

	_, height := screen.Size()
	frame := make([]string, height)
	for row := range height {
		frame[row] = strings.TrimRight(rowToString(screen.GetRow(row)), " ")
	}

	screen.frameLock.Lock()
	defer screen.frameLock.Unlock()
	screen.frames = append(screen.frames, frame)
}

// Rows of the most recently shown frame
func (screen *pagingLoopScreen) shownRows() []string {
	screen.frameLock.Lock()
	defer screen.frameLock.Unlock()
	if len(screen.frames) == 0 {
		return nil
	}
	return screen.frames[len(screen.frames)-1]
}

// All frames shown so far, oldest first
func (screen *pagingLoopScreen) shownFrames() [][]string {
	screen.frameLock.Lock()
	defer screen.frameLock.Unlock()
	return slices.Clone(screen.frames)
}

// Wait until condition is true, however long that takes. waitingFor() says what
// we are waiting for, and is called only when reporting that we still are.
func waitUntil(t *testing.T, condition func() bool, waitingFor func() string) {
	t.Helper()

	nextReport := time.Now().Add(stillWaitingInterval)
	for !condition() {
		if time.Now().After(nextReport) {
			reportStillWaiting(t, "%s", waitingFor())
			nextReport = time.Now().Add(stillWaitingInterval)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Wait until some shown row says exactly this, then return all shown rows
func (screen *pagingLoopScreen) waitForRow(t *testing.T, want string) []string {
	t.Helper()

	var rows []string
	waitUntil(t, func() bool {
		rows = screen.shownRows()
		return slices.Contains(rows, want)
	}, func() string {
		return fmt.Sprintf("%q on screen, last frame:\n%s", want, strings.Join(rows, "\n"))
	})

	return rows
}

// Start paging in the background, and stop paging again when the test ends.
//
// Returns a channel that is closed once paging is done.
//
// Stopping matters even for tests that expect the pager to quit on its own: a
// pager left running keeps painting on its spinner ticks for as long as the test
// binary lives. Same thing with readers left open, they keep tailing their
// files.
func startPagingInBackground(t *testing.T, pager *Pager, screen *pagingLoopScreen) chan struct{} {
	t.Helper()
	isolateStyles(t)

	pagingDone := make(chan struct{})
	go func() {
		pager.StartPaging(screen, colorlessChromaStyle, formatters.TTY16m)
		close(pagingDone)
	}()

	t.Cleanup(func() {
		select {
		case screen.events <- twin.EventExit{}:
		case <-pagingDone:
		}

		<-pagingDone
		pager.CloseReaders()
	})

	return pagingDone
}

// Hand an event to the pager, and wait for the pager to take it.
//
// Fails the test if the pager quits instead.
func handEvent(t *testing.T, screen *pagingLoopScreen, pagingDone chan struct{}, event twin.Event) {
	t.Helper()

	stillWaiting := time.NewTicker(stillWaitingInterval)
	defer stillWaiting.Stop()

	for {
		select {
		case screen.events <- event:
			return

		case <-pagingDone:
			t.Fatal("Pager quit instead of taking our event")

		case <-stillWaiting.C:
			reportStillWaiting(t, "the pager to take event %#v", event)
		}
	}
}

func typeRunes(t *testing.T, screen *pagingLoopScreen, pagingDone chan struct{}, text string) {
	t.Helper()

	for _, char := range text {
		handEvent(t, screen, pagingDone, twin.EventRune{Rune: char})
	}
}
