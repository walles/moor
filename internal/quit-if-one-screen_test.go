package internal

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/walles/moor/v2/internal/reader"
	"github.com/walles/twin"
	"gotest.tools/v3/assert"
)

// How long to wait for the pager to get somewhere. Long enough that exceeding it
// means the pager is stuck rather than slow.
const pagerTimeout = time.Second

// An event the pager has no handling for.
//
// Sending one on an unbuffered event channel completes only once the main loop
// receives it, and the loop reads events after deciding whether to paint. So a
// completed send means the decision has been made.
type nudge struct{}

// A screen that can run the pager main loop, and that counts paints.
//
// twin.FakeScreen.Events() returns nil, and receiving from a nil channel blocks
// forever, so the main loop cannot be run with a plain FakeScreen.
//
// Show() is what takes over the user's terminal, alternate screen included, so
// counting Show() calls is counting how much the user gets to see.
type countingScreen struct {
	*twin.FakeScreen
	events chan twin.Event
	shows  atomic.Int64
}

func newCountingScreen(width int, height int) *countingScreen {
	return &countingScreen{
		FakeScreen: twin.NewFakeScreen(width, height),

		// Unbuffered, so that sending on it is a handshake with the main loop
		events: make(chan twin.Event),
	}
}

func (screen *countingScreen) Events() chan twin.Event {
	return screen.events
}

func (screen *countingScreen) Show() {
	screen.shows.Add(1)
	screen.FakeScreen.Show()
}

// Start paging in the background, and stop paging again when the test ends.
//
// Returns a channel that is closed once paging is done.
//
// Stopping matters even for tests that expect the pager to quit on its own: a
// pager left running keeps painting on its spinner ticks for as long as the test
// binary lives.
func startPagingInBackground(t *testing.T, pager *Pager, screen *countingScreen) chan struct{} {
	t.Helper()
	isolateStyles(t)

	pagingDone := make(chan struct{})
	go func() {
		pager.StartPaging(screen, nil, nil)
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
func handEvent(t *testing.T, screen *countingScreen, pagingDone chan struct{}, event twin.Event) {
	t.Helper()

	select {
	case screen.events <- event:

	case <-pagingDone:
		t.Fatal("Pager quit instead of taking our event")

	case <-time.After(pagerTimeout):
		t.Fatal("Pager never got as far as reading events")
	}
}

// Report whether the pager has painted anything, once it has decided whether to.
//
// The pager takes events only after deciding, so handing it an event it ignores
// and waiting for that to be taken puts the decision behind us.
//
// Only ever asks about paints up to that decision. A caller that hands over
// something the pager reacts to must ask before handing it over, not after: this
// answer is read after the pager is free to run on, so a later lap's paint can
// still land in it.
func hasPainted(t *testing.T, screen *countingScreen, pagingDone chan struct{}) bool {
	t.Helper()

	handEvent(t, screen, pagingDone, nudge{})

	return screen.shows.Load() > 0
}

// With --quit-if-one-screen, contents that fit must never be painted, not even
// while we are still waiting to find out whether they fit.
//
// Highlighting can change the answer, so until it is done we don't know. Ask the
// user's terminal for nothing in the meantime: painting first and quitting right
// after is the blink from https://github.com/walles/moor/issues/425.
func TestQuitIfOneScreenPaintsNothingWhileHighlighting(t *testing.T) {
	testReader := reader.NewFromTextForTesting("", "hello\nworld")

	// NewFromTextForTesting() leaves this nil, and the pager needs it to hear
	// about highlighting finishing. Assign before paging starts, the pager reads
	// it from a goroutine of its own.
	testReader.MaybeDone = make(chan bool, 2)

	// Highlighting pending, like for a source file argument or piped JSON
	testReader.HighlightingDone.Store(false)

	screen := newCountingScreen(20, 10)

	pager := NewPager(testReader)
	pager.QuitIfOneScreen = true

	pagingDone := startPagingInBackground(t, pager, screen)

	painted := hasPainted(t, screen, pagingDone)
	assert.Assert(t, !painted,
		"Nothing should be painted while we don't know whether the contents fit")

	// Now we know, and two lines do fit on ten, so the pager should quit
	testReader.HighlightingDone.Store(true)
	testReader.MaybeDone <- true

	select {
	case <-pagingDone:
	case <-time.After(pagerTimeout):
		t.Fatal("Pager should have quit, two lines fit on ten")
	}
}

// Contents that do not fit must be painted right away, without waiting for
// highlighting.
//
// Highlighting only ever gets us more lines to show, so contents that are
// already too tall are staying too tall. We know we are staying, and waiting
// would be latency in front of the first paint for nothing.
func TestQuitIfOneScreenPaintsContentsThatDoNotFit(t *testing.T) {
	testReader := reader.NewFromTextForTesting("", strings.Repeat("hello\n", 20))
	testReader.MaybeDone = make(chan bool, 2)
	testReader.HighlightingDone.Store(false)

	screen := newCountingScreen(20, 10)

	pager := NewPager(testReader)
	pager.QuitIfOneScreen = true

	pagingDone := startPagingInBackground(t, pager, screen)

	painted := hasPainted(t, screen, pagingDone)
	assert.Assert(t, painted,
		"Contents that don't fit should be painted without waiting for highlighting")
}

// Waiting for highlighting must never cost the user their session.
//
// Highlighting is not guaranteed to finish: a panic while highlighting is only
// logged, and leaves it pending forever. So the wait lasts only until the user
// does something, and then they get their contents.
func TestQuitIfOneScreenPaintsForAUserWhoAsks(t *testing.T) {
	testReader := reader.NewFromTextForTesting("", "hello\nworld")
	testReader.MaybeDone = make(chan bool, 2)

	// Highlighting that never finishes, like after a panic while highlighting
	testReader.HighlightingDone.Store(false)

	screen := newCountingScreen(20, 10)

	pager := NewPager(testReader)
	pager.QuitIfOneScreen = true

	pagingDone := startPagingInBackground(t, pager, screen)

	painted := hasPainted(t, screen, pagingDone)
	assert.Assert(t, !painted, "Nothing should be painted before the user asks")

	// A mouse event with no buttons set: the pager has nothing to do about it
	// beyond noticing that somebody is there.
	handEvent(t, screen, pagingDone, twin.EventMouse{})

	painted = hasPainted(t, screen, pagingDone)
	assert.Assert(t, painted, "A user who asks should get the contents, highlighted or not")
}
