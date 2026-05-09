package reader

import (
	"os"
	"testing"
	"time"

	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/walles/moor/v2/internal/linemetadata"
	"gotest.tools/v3/assert"
)

func setupWatcherTest(t *testing.T, initialContents string) (*ReaderImpl, *os.File) {
	t.Helper()
	file, err := os.CreateTemp("", "moor-watcher-test-*.txt")
	assert.NilError(t, err)
	t.Cleanup(func() { _ = os.Remove(file.Name()) })

	if initialContents != "" {
		_, err = file.WriteString(initialContents)
		assert.NilError(t, err)
	}

	testMe, err := NewFromFilename(file.Name(), formatters.TTY16m, ReaderOptions{Style: styles.Get("native")})
	assert.NilError(t, err)
	assert.NilError(t, testMe.Wait())

	return testMe, file
}

func waitForLineCount(t *testing.T, testMe *ReaderImpl, expectedCount int) {
	t.Helper()
	waitForCondition(t, func() bool {
		allLines := testMe.GetLines(linemetadata.Index{}, expectedCount+10)
		return len(allLines.Lines) == expectedCount
	}, "waiting for line count")
}

func waitForCondition(t *testing.T, condition func() bool, errMsg string) {
	t.Helper()
	for range 20 {
		if condition() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out: %s", errMsg)
}

func assertLines(t *testing.T, testMe *ReaderImpl, expectedLines ...string) {
	t.Helper()
	allLines := testMe.GetLines(linemetadata.Index{}, len(expectedLines)+10)
	assert.Equal(t, len(allLines.Lines), len(expectedLines), "Expected %d lines, got %d", len(expectedLines), len(allLines.Lines))

	for i, expected := range expectedLines {
		assert.Equal(t, allLines.Lines[i].Plain(), expected)
	}
}

func TestReadUpdatingFile(t *testing.T) {
	const firstLineString = "First line\n"
	testMe, file := setupWatcherTest(t, firstLineString)

	assert.Equal(t, int(testMe.bytesCount), len([]byte(firstLineString)))
	assertLines(t, testMe, "First line")

	const secondLineString = "Second line\n"
	_, err := file.WriteString(secondLineString)
	assert.NilError(t, err)

	waitForLineCount(t, testMe, 2)
	assertLines(t, testMe, "First line", "Second line")
	assert.Equal(t, int(testMe.bytesCount), len([]byte(firstLineString+secondLineString)))

	const thirdLineString = "Third line\n"
	_, err = file.WriteString(thirdLineString)
	assert.NilError(t, err)

	waitForLineCount(t, testMe, 3)
	assertLines(t, testMe, "First line", "Second line", "Third line")
	assert.Equal(t, int(testMe.bytesCount), len([]byte(firstLineString+secondLineString+thirdLineString)))
}

// If a file is rewritten (shrunk and replaced with new content), tailing should
// detect the shrink and reload the file rather than giving up.
func TestReadShrunkFile(t *testing.T) {
	testMe, file := setupWatcherTest(t, "First line\n")
	assertLines(t, testMe, "First line")

	// Rewrite the file with shorter content, so the file shrinks
	err := os.WriteFile(file.Name(), []byte("New\n"), 0600)
	assert.NilError(t, err)

	waitForCondition(t, func() bool {
		allLines := testMe.GetLines(linemetadata.Index{}, 10)
		return len(allLines.Lines) == 1 && allLines.Lines[0].Plain() == "New"
	}, "waiting for shrunk file to reload with 'New'")

	assert.NilError(t, testMe.Wait())
	assertLines(t, testMe, "New")
}

// If people keep appending to the currently opened file we should display those
// changes.
//
// This test verifies it with an initially empty file.
func TestReadUpdatingFile_InitiallyEmpty(t *testing.T) {
	testMe, file := setupWatcherTest(t, "")
	assertLines(t, testMe)

	_, err := file.WriteString("Text\n")
	assert.NilError(t, err)

	waitForLineCount(t, testMe, 1)
	assertLines(t, testMe, "Text")
}

// If people keep appending to the currently opened file we should display those
// changes.
//
// This test verifies it with the initial contents not ending with a linefeed.
func TestReadUpdatingFile_HalfLine(t *testing.T) {
	testMe, file := setupWatcherTest(t, "Start")
	assert.Equal(t, int(testMe.bytesCount), len([]byte("Start")))

	const secondLineString = ", end\n"
	_, err := file.WriteString(secondLineString)
	assert.NilError(t, err)

	waitForCondition(t, func() bool {
		allLines := testMe.GetLines(linemetadata.Index{}, 10)
		return len(allLines.Lines) == 1 && allLines.Lines[0].Plain() == "Start, end"
	}, "waiting for line to update")

	assertLines(t, testMe, "Start, end")
	assert.Equal(t, int(testMe.bytesCount), len([]byte("Start, end\n")))
}

// If people keep appending to the currently opened file we should display those
// changes.
//
// This test verifies it with the initial contents ending in the middle of an UTF-8 character.
func TestReadUpdatingFile_HalfUtf8(t *testing.T) {
	// Write "h" and half an "ä" to the file
	testMe, file := setupWatcherTest(t, string([]byte("här")[0:2]))
	assert.Equal(t, testMe.GetLineCount(), 1)

	// Append the rest of the UTF-8 character
	_, err := file.WriteString("här"[2:])
	assert.NilError(t, err)

	waitForCondition(t, func() bool {
		allLines := testMe.GetLines(linemetadata.Index{}, 10)
		return len(allLines.Lines) == 1 && allLines.Lines[0].Plain() == "här"
	}, "waiting for utf-8 char to complete")

	assertLines(t, testMe, "här")
	assert.Equal(t, int(testMe.bytesCount), len([]byte("här")))
}

// If a file is completely rewritten and ends up not smaller than before, but clearly
// has different boundary bytes (not an append), tailing should detect the
// replacement and reload instead of attempting to append.
func TestReadRewrittenFile_Replaced(t *testing.T) {
	testMe, file := setupWatcherTest(t, "First\nSecond\n")
	assertLines(t, testMe, "First", "Second")

	const replacementRow = "Totally different data replaces the whole file\n"
	err := os.WriteFile(file.Name(), []byte(replacementRow), 0600)
	assert.NilError(t, err)

	waitForCondition(t, func() bool {
		allLines := testMe.GetLines(linemetadata.Index{}, 10)
		return len(allLines.Lines) == 1 && allLines.Lines[0].Plain() == "Totally different data replaces the whole file"
	}, "waiting for rewrite to reload")

	assert.NilError(t, testMe.Wait())
	assertLines(t, testMe, "Totally different data replaces the whole file")
}

type fakeFileInfo struct {
	size    int64
	modTime time.Time
}

func (f fakeFileInfo) Name() string       { return "test.txt" }
func (f fakeFileInfo) Size() int64        { return f.size }
func (f fakeFileInfo) Mode() os.FileMode  { return 0644 }
func (f fakeFileInfo) ModTime() time.Time { return f.modTime }
func (f fakeFileInfo) IsDir() bool        { return false }
func (f fakeFileInfo) Sys() any           { return nil }

func TestDetermineTailAction(t *testing.T) {
	t0 := time.Now()
	t1 := t0.Add(1 * time.Second)

	const (
		smaller int64 = 100
		larger  int64 = 2000
	)

	tests := []struct {
		name         string
		isCompressed bool
		oldSize      int64
		oldModTime   time.Time
		newSize      int64
		newModTime   time.Time
		statErr      error
		expected     tailAction
	}{
		{"compressed file grown reloads", true, smaller, t0, larger, t0, nil, tailActionReload},
		{"compressed file shrunk reloads", true, larger, t0, smaller, t0, nil, tailActionReload},
		{"compressed file same size updated timestamp reloads", true, smaller, t0, smaller, t1, nil, tailActionReload},
		{"compressed file unchanged continues", true, smaller, t0, smaller, t0, nil, tailActionContinue},
		{"stat error stops tailing", false, smaller, t0, smaller, t0, os.ErrNotExist, tailActionStop},
		{"unknown previous stat stops tailing", false, -1, t0, smaller, t0, nil, tailActionStop},
		{"unchanged file continues tailing", false, smaller, t0, smaller, t0, nil, tailActionContinue},
		{"same size updated timestamp reloads", false, smaller, t0, smaller, t1, nil, tailActionReload},
		{"shrunk file reloads", false, larger, t0, smaller, t0, nil, tailActionReload},
		{"grown file appends", false, smaller, t0, larger, t0, nil, tailActionAppend},
		{"grown file updated timestamp appends", false, smaller, t0, larger, t1, nil, tailActionAppend},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var oldStat os.FileInfo
			if tt.oldSize != -1 {
				oldStat = fakeFileInfo{size: tt.oldSize, modTime: tt.oldModTime}
			}

			actual := determineTailAction(
				"test.txt",
				tt.isCompressed,
				oldStat,
				fakeFileInfo{size: tt.newSize, modTime: tt.newModTime},
				tt.statErr,
				nil, // headerBytes
			)
			assert.Equal(t, actual, tt.expected)
		})
	}
}
