package reader

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/walles/moor/v2/internal/linemetadata"
	"github.com/walles/moor/v2/internal/reader/readsummary"
	"gotest.tools/v3/assert"
)

// TestMain overrides tailFile()'s poll interval for the whole package's test
// binary, so tests waiting on tailing don't have to sleep at the production
// 1s cadence. This is set once, before any test (and thus any tailing
// goroutine) starts, so there's no data race with tailFile()'s reads of it.
func TestMain(m *testing.M) {
	tailPollInterval = 10 * time.Millisecond
	sameFile = func(oldStat, newStat os.FileInfo) bool {
		if _, isFake := oldStat.(fakeFileInfo); isFake {
			return true
		}
		return os.SameFile(oldStat, newStat)
	}
	os.Exit(m.Run())
}

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

// Wait for testMe to ask for a reload, then do what the pager would do and
// return a fresh clone of it
func waitForReload(t *testing.T, testMe *ReaderImpl) *ReaderImpl {
	t.Helper()
	waitForCondition(t, testMe.WantsReload, "waiting for a reload request")

	clone, err := testMe.Clone()
	assert.NilError(t, err)
	t.Cleanup(clone.Close)
	assert.NilError(t, clone.Wait())

	return clone
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

	assert.Equal(t, int(testMe.readSoFar.Count()), len([]byte(firstLineString)))
	assertLines(t, testMe, "First line")

	const secondLineString = "Second line\n"
	_, err := file.WriteString(secondLineString)
	assert.NilError(t, err)

	waitForLineCount(t, testMe, 2)
	assertLines(t, testMe, "First line", "Second line")
	assert.Equal(t, int(testMe.readSoFar.Count()), len([]byte(firstLineString+secondLineString)))

	const thirdLineString = "Third line\n"
	_, err = file.WriteString(thirdLineString)
	assert.NilError(t, err)

	waitForLineCount(t, testMe, 3)
	assertLines(t, testMe, "First line", "Second line", "Third line")
	assert.Equal(t, int(testMe.readSoFar.Count()), len([]byte(firstLineString+secondLineString+thirdLineString)))
}

// If a file is rewritten (shrunk and replaced with new content), tailing should
// detect the shrink and reload the file rather than giving up.
func TestReadShrunkFile(t *testing.T) {
	testMe, file := setupWatcherTest(t, "First line\n")
	assertLines(t, testMe, "First line")

	// Rewrite the file with shorter content, so the file shrinks
	err := os.WriteFile(file.Name(), []byte("New\n"), 0600)
	assert.NilError(t, err)

	assertLines(t, waitForReload(t, testMe), "New")
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
	assert.Equal(t, int(testMe.readSoFar.Count()), len([]byte("Start")))

	const secondLineString = ", end\n"
	_, err := file.WriteString(secondLineString)
	assert.NilError(t, err)

	waitForCondition(t, func() bool {
		allLines := testMe.GetLines(linemetadata.Index{}, 10)
		return len(allLines.Lines) == 1 && allLines.Lines[0].Plain() == "Start, end"
	}, "waiting for line to update")

	assertLines(t, testMe, "Start, end")
	assert.Equal(t, int(testMe.readSoFar.Count()), len([]byte("Start, end\n")))
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
	assert.Equal(t, int(testMe.readSoFar.Count()), len([]byte("här")))
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

	assertLines(t, waitForReload(t, testMe), "Totally different data replaces the whole file")
}

// If a small file is appended to and then rewritten in place, starting with
// the original contents and growing, tailing should reload rather than append.
func TestReadRewrittenFile_AfterAppend(t *testing.T) {
	testMe, file := setupWatcherTest(t, "a\n")

	_, err := file.WriteString("b\n")
	assert.NilError(t, err)
	waitForLineCount(t, testMe, 2)
	waitForCondition(t, func() bool {
		testMe.RLock()
		defer testMe.RUnlock()
		return testMe.lastStat.Size() == 4
	}, "waiting for the append to be noticed")

	_, err = file.WriteAt([]byte("a\nc\nd\n"), 0)
	assert.NilError(t, err)

	assertLines(t, waitForReload(t, testMe), "a", "c", "d")
}

// The first line of these is longer than the header we track, so only the
// bytes after it can tell the files apart
var longFirstLine = strings.Repeat("x", 2000)
var originalContents = longFirstLine + "\nold\n"
var replacementContents = longFirstLine + "\nnew\nlonger\n"

// If a file is rewritten in place, starting with the same bytes and growing,
// tailing should reload rather than append.
//
// Ref: https://github.com/walles/moor/pull/479
func TestReadRewrittenFile_SameStart(t *testing.T) {
	testMe, file := setupWatcherTest(t, originalContents)
	assertLines(t, testMe, longFirstLine, "old")

	_, err := file.WriteAt([]byte(replacementContents), 0)
	assert.NilError(t, err)

	assertLines(t, waitForReload(t, testMe), longFirstLine, "new", "longer")
}

// If another file is moved into place, starting with the same bytes and being
// longer, tailing should reload rather than append.
//
// Ref: https://github.com/walles/moor/pull/479
func TestReadReplacedFile_SameStart(t *testing.T) {
	testMe, file := setupWatcherTest(t, originalContents)
	assertLines(t, testMe, longFirstLine, "old")

	// Windows refuses renaming over files that are open
	assert.NilError(t, file.Close())

	replacement := file.Name() + ".new"
	err := os.WriteFile(replacement, []byte(replacementContents), 0600)
	assert.NilError(t, err)
	t.Cleanup(func() { _ = os.Remove(replacement) })
	assert.NilError(t, os.Rename(replacement, file.Name()))

	assertLines(t, waitForReload(t, testMe), longFirstLine, "new", "longer")
}

// Appending to a file must be taken for an append. Reloading would show the
// same lines, so the tests that only check lines can't tell these apart.
func TestDetermineTailAction_RealAppend(t *testing.T) {
	// Longer than the header and the trailer together
	contents := []byte(strings.Repeat("0123456789\n", 300))

	for _, appended := range []string{"x", "More lines\nAnd more\n"} {
		t.Run(appended, func(t *testing.T) {
			fileName := filepath.Join(t.TempDir(), "file.txt")
			assert.NilError(t, os.WriteFile(fileName, contents, 0600))

			// Read in two chunks with the trailer spanning both of them
			var readSoFar readsummary.Summary
			split := len(contents) - 500
			readSoFar.Add(contents[:split])
			readSoFar.Add(contents[split:])

			oldStat, err := os.Stat(fileName)
			assert.NilError(t, err)

			file, err := os.OpenFile(fileName, os.O_APPEND|os.O_WRONLY, 0)
			assert.NilError(t, err)
			_, err = file.WriteString(appended)
			assert.NilError(t, err)
			assert.NilError(t, file.Close())

			newStat, err := os.Stat(fileName)
			assert.NilError(t, err)

			action := determineTailAction(fileName, false, oldStat, newStat, nil, &readSoFar)
			assert.Equal(t, action, tailActionAppend)
		})
	}
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
				&readsummary.Summary{},
			)
			assert.Equal(t, actual, tt.expected)
		})
	}
}

// Repro for https://github.com/walles/moor/issues/435
func TestDetermineTailAction_gzippedUnchanged(t *testing.T) {
	gzippedName := filepath.Join("..", "..", "sample-files", "compressed.txt.gz")

	testMe, err := NewFromFilename(gzippedName,
		formatters.TTY, ReaderOptions{Style: styles.Get("native")})
	assert.NilError(t, err)

	assert.NilError(t, testMe.Wait())

	assert.Equal(t, len(testMe.readSoFar.Header()) > 0, true)

	action := determineTailAction(
		gzippedName,
		true, // isCompressed
		fakeFileInfo{size: 1234, modTime: time.Unix(1000, 0)},
		fakeFileInfo{size: 1234, modTime: time.Unix(1000, 0)},
		nil, // statErr
		&testMe.readSoFar,
	)

	assert.Equal(t, action, tailActionContinue)
}

// A file that another file was moved into place over must be reloaded, also
// when it looks like the old file with more data at the end.
func TestDetermineTailAction_replacedByLargerFile(t *testing.T) {
	fileName := filepath.Join(t.TempDir(), "test.txt")
	err := os.WriteFile(fileName, []byte("old\n"), 0600)
	assert.NilError(t, err)

	// Stat the opened file like the reader does. On Windows, a stat of the file
	// name would get its file ID only when compared, after the rename below.
	stream, _, oldStat, err := zOpenWithStat(fileName)
	assert.NilError(t, err)
	assert.Assert(t, oldStat != nil)
	assert.NilError(t, stream.Close())

	err = os.WriteFile(fileName+".new", []byte("old\nnew\n"), 0600)
	assert.NilError(t, err)
	err = os.Rename(fileName+".new", fileName)
	assert.NilError(t, err)
	newStat, err := os.Stat(fileName)
	assert.NilError(t, err)

	var readSoFar readsummary.Summary
	readSoFar.Add([]byte("old\n"))

	action := determineTailAction(
		fileName,
		false, // isCompressed
		oldStat,
		newStat,
		nil, // statErr
		&readSoFar,
	)

	assert.Equal(t, action, tailActionReload)
}
