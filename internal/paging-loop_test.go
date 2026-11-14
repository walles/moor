package internal

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/walles/twin"
	"gotest.tools/v3/assert"

	"github.com/walles/moor/v2/internal/reader"
)

// A fake screen that can drive the real StartPaging() loop. twin.FakeScreen
// has no event channel, and its cells may only be touched from the pager's
// main loop. So we add the channel, and record each frame as it is shown.
type pagingLoopScreen struct {
	*twin.FakeScreen
	events chan twin.Event

	lock  sync.Mutex
	frame []string // Rows of the most recently shown frame
}

func (s *pagingLoopScreen) Events() chan twin.Event {
	return s.events
}

// Called from the pager's main loop after each redraw
func (s *pagingLoopScreen) Show() {
	_, height := s.Size()
	frame := make([]string, height)
	for row := range height {
		frame[row] = strings.TrimRight(rowToString(s.GetRow(row)), " ")
	}

	s.lock.Lock()
	defer s.lock.Unlock()
	s.frame = frame
}

func (s *pagingLoopScreen) shownRows() []string {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.frame
}

func (s *pagingLoopScreen) typeRunes(text string) {
	for _, char := range text {
		s.events <- twin.EventRune{Rune: char}
	}
}

// Wait until some shown row says exactly this, then return all shown rows
func (s *pagingLoopScreen) waitForRow(t *testing.T, want string) []string {
	t.Helper()

	giveUp := time.Now().Add(5 * time.Second)
	for time.Now().Before(giveUp) {
		rows := s.shownRows()
		for _, row := range rows {
			if row == want {
				return rows
			}
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("Never saw %q on screen, last frame:\n%s", want, strings.Join(s.shownRows(), "\n"))
	return nil
}

// Run the real pager loop until the test ends
func startPagingLoop(t *testing.T, pager *Pager) *pagingLoopScreen {
	t.Helper()

	screen := &pagingLoopScreen{
		FakeScreen: twin.NewFakeScreen(20, 10),
		events:     make(chan twin.Event, 100),
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		pager.StartPaging(screen, colorlessChromaStyle, formatters.TTY16m)
	}()

	t.Cleanup(func() {
		screen.events <- twin.EventExit{}
		<-done
	})

	return screen
}

// Following must keep working in an input we switched to, which needs the
// background goroutine to start listening to the new input's lines.
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
	pager.Follow = true
	screen := startPagingLoop(t, pager)
	screen.waitForRow(t, "a0")

	// The background goroutine throttles more-lines notifications for 200ms,
	// and picks up the current input when that throttling ends. Switch after
	// that, or this test passes even if switching never tells the goroutine.
	time.Sleep(300 * time.Millisecond)

	screen.typeRunes(":n")
	screen.waitForRow(t, "b0")

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
	for !r.HighlightingDone.Load() {
		// Reloading is ignored until highlighting is done
		<-r.MaybeDone
	}

	pager := NewPager(r)
	pager.ShowLineNumbers = false
	screen := startPagingLoop(t, pager)

	screen.typeRunes("&bepa")
	screen.events <- twin.EventKeyCode{KeyCode: twin.KeyEnter}
	rows := screen.waitForRow(t, "bepa")
	assert.Equal(t, rows[0], "bepa")

	// Change the file so we can tell when the reload shows up
	assert.NilError(t, os.WriteFile(file, []byte("apa\nbepa2\ncepa\n"), 0o600))
	screen.typeRunes("r")

	rows = screen.waitForRow(t, "bepa2")
	assert.Equal(t, rows[0], "bepa2", "Filter should survive reload")
}
