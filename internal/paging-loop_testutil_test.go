package internal

import (
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/walles/twin"
)

// How long to wait for the pager to get somewhere. Long enough that exceeding it
// means the pager is stuck rather than slow.
const pagerTimeout = time.Second

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
	frame     []string // Rows of the most recently shown frame
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
	screen.frame = frame
}

func (screen *pagingLoopScreen) shownRows() []string {
	screen.frameLock.Lock()
	defer screen.frameLock.Unlock()
	return screen.frame
}

// Wait until some shown row says exactly this, then return all shown rows
func (screen *pagingLoopScreen) waitForRow(t *testing.T, want string) []string {
	t.Helper()

	giveUp := time.Now().Add(pagerTimeout)
	for time.Now().Before(giveUp) {
		rows := screen.shownRows()
		for _, row := range rows {
			if row == want {
				return rows
			}
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("Never saw %q on screen, last frame:\n%s", want, strings.Join(screen.shownRows(), "\n"))
	return nil
}

// Start paging in the background, and stop paging again when the test ends.
//
// Returns a channel that is closed once paging is done.
//
// Stopping matters even for tests that expect the pager to quit on its own: a
// pager left running keeps painting on its spinner ticks for as long as the test
// binary lives.
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
	})

	return pagingDone
}

// Hand an event to the pager, and wait for the pager to take it.
//
// Fails the test rather than hanging if the pager is not taking events.
func handEvent(t *testing.T, screen *pagingLoopScreen, pagingDone chan struct{}, event twin.Event) {
	t.Helper()

	select {
	case screen.events <- event:

	case <-pagingDone:
		t.Fatal("Pager quit instead of taking our event")

	case <-time.After(pagerTimeout):
		t.Fatal("Pager never got as far as reading events")
	}
}

func typeRunes(t *testing.T, screen *pagingLoopScreen, pagingDone chan struct{}, text string) {
	t.Helper()

	for _, char := range text {
		handEvent(t, screen, pagingDone, twin.EventRune{Rune: char})
	}
}
