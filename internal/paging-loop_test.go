package internal

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	for !r.HighlightingDone.Load() {
		// Reloading is ignored until highlighting is done
		<-r.MaybeDone
	}

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
