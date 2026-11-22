package reader

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/walles/moor/v2/internal/linemetadata"
	"gotest.tools/v3/assert"
)

func TestPauseAfterNLines(t *testing.T) {
	pauseAfterLines := 1

	// Get ourselves a reader
	twoLines := strings.NewReader("one\ntwo\n")
	testMe, err := NewFromStream(
		"TestPauseAfterNLines",
		twoLines,
		formatters.TTY,
		ReaderOptions{
			PauseAfterLines: &pauseAfterLines,
			Style:           styles.Get("native"),
		})
	assert.NilError(t, err)

	// Expect pause since we configured the reader to pause after 1 line ^
	for testMe.PauseStatus.Load() != true {
	}
	assert.Assert(t, testMe.PauseStatus.Load() == true,
		"Reader should be paused after reading %d lines", pauseAfterLines)

	// Verify that we have *not* received a done notification yet
	assert.Assert(t, testMe.ReadingDone() == false,
		"Reader should not be done yet, only paused")

	// Check that the reader has exactly the first line and nothing else
	lines := testMe.GetLines(linemetadata.Index{}, 2).Lines
	assert.Equal(t, len(lines), 1,
		"Reader should have exactly one line after pausing")
	assert.Equal(t, lines[0].Plain(), "one",
		"Reader should have the first line after pausing")

	// Tell reader to continue
	testMe.SetPauseAfterLines(99)

	// More lines allowed, expect pause to be over
	for testMe.PauseStatus.Load() != false {
	}
	assert.Assert(t, testMe.PauseStatus.Load() == false,
		"Reader should be unpaused after continuing")

	// Expect a done notification
	<-testMe.MaybeDone
	assert.Assert(t, testMe.ReadingDone() == true,
		"Reader should be done after reading all lines")

	// Check that the reader has both lines
	lines = testMe.GetLines(linemetadata.Index{}, 3).Lines
	assert.Equal(t, len(lines), 2,
		"Reader should have two lines after unpausing")
	assert.Equal(t, lines[0].Plain(), "one",
		"Reader should have the first line after unpausing")
	assert.Equal(t, lines[1].Plain(), "two",
		"Reader should have the second line after unpausing")
}

// While the reader is paused during initial loading, the line count is hidden
// from the status text (it would be misleading, since it's not the final count
// yet). The filename and the percentage should still be visually separated from
// each other in that case.
func TestStatusSeparatorWhilePaused(t *testing.T) {
	pauseAfterLines := 1

	twoLines := strings.NewReader("one\ntwo\n")
	testMe, err := NewFromStream(
		"large-git-log-patch.txt",
		twoLines,
		formatters.TTY,
		ReaderOptions{
			PauseAfterLines: &pauseAfterLines,
			Style:           styles.Get("native"),
		})
	assert.NilError(t, err)

	// Expect pause since we configured the reader to pause after 1 line ^
	for testMe.PauseStatus.Load() != true {
	}
	assert.Assert(t, testMe.ReadingDone() == false,
		"Reader should not be done yet, only paused")

	status := testMe.GetLines(linemetadata.Index{}, 2)
	assert.Equal(t, status.FilenameText, "large-git-log-patch.txt")
	assert.Equal(t, status.StatusText, ": 100%")
}

// Test pausing behavior after we're done reading from a file, and then another line is added.
func TestPauseAfterNLines_Polling(t *testing.T) {
	pauseAfterLines := 1

	// Create a file with a line in it
	file, err := os.CreateTemp("", "TestPauseAfterNLines_Polling")
	assert.NilError(t, err)
	defer os.Remove(file.Name()) //nolint:errcheck
	_, err = file.WriteString("one\n")
	assert.NilError(t, err)

	// Point a reader at the file
	testMe, err := NewFromFilename(file.Name(), formatters.TTY, ReaderOptions{
		PauseAfterLines: &pauseAfterLines,
		Style:           styles.Get("native"),
	})
	assert.NilError(t, err)

	// Expect pause since we configured the reader to pause after 1 line ^
	for testMe.PauseStatus.Load() != true {
	}
	assert.Assert(t, testMe.PauseStatus.Load() == true,
		"Reader should be paused after reading %d lines", pauseAfterLines)

	// Verify state before we add another line to the file
	lines := testMe.GetLines(linemetadata.Index{}, 2).Lines
	assert.Equal(t, len(lines), 1,
		"Reader should have exactly one line after pausing")
	assert.Equal(t, lines[0].Plain(), "one",
		"Reader should have the first line after pausing")

	// Write another line to the file
	_, err = file.WriteString("two\n")
	assert.NilError(t, err)

	// Wait for several tailFile() poll cycles to give us the new line even
	// though we are paused. That shouldn't happen. If it does we fail here.
	for range 20 {
		allLines := testMe.GetLines(linemetadata.Index{}, 10)
		if len(allLines.Lines) == 2 {
			assert.Assert(t, false, "Reader should not have received a new line while paused")
		}
		time.Sleep(tailPollInterval)
	}

	// No new line while paused, good! Unpause.
	testMe.SetPauseAfterLines(99)

	// Give the new line a bunch of poll cycles to arrive
	var bothLines []NumberedLine
	for range 20 {
		bothLines = testMe.GetLines(linemetadata.Index{}, 10).Lines
		if len(bothLines) > 1 {
			break
		}
		time.Sleep(tailPollInterval)
	}

	// Verify that we have both lines now
	assert.Equal(t, len(bothLines), 2,
		"Reader should have two lines after unpausing")
	assert.Equal(t, bothLines[0].Plain(), "one",
		"Reader should have the first line after unpausing")
	assert.Equal(t, bothLines[1].Plain(), "two",
		"Reader should have the second line after unpausing")
}
