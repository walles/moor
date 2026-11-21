package reader

// This file contains the logic for file watching and tailing appended bytes.

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/walles/moor/v2/internal/reader/readsummary"
)

// How often tailFile() checks the tailed file for changes. Overridden in
// tests to make them run faster.
var tailPollInterval = time.Second

// Overridden in tests, os.SameFile() doesn't work on the test fakes.
var sameFile = os.SameFile

type tailAction int

const (
	tailActionStop     tailAction = iota // Give up on tailing
	tailActionContinue                   // Nothing happened, keep tailing
	tailActionReload                     // File was rewritten, reload from the beginning
	tailActionAppend                     // File was appended to, read the new contents
)

// requestReload tells the pager this reader has gone stale and needs to be
// replaced by a fresh one.
func (reader *ReaderImpl) requestReload() {
	reader.reloadWanted.Store(true)

	// The pager looks for reload requests when told about new lines
	select {
	case reader.MoreLinesAdded <- true:
	default:
	}
}

// WantsReload is true if the file was rewritten after we read it. The pager
// should then replace this reader with a clone of it.
func (reader *ReaderImpl) WantsReload() bool {
	return reader.reloadWanted.Load()
}

// readNewBytes reads bytes appended to the file since we last read it.
//
// Returns (shouldContinue, error): shouldContinue=false means tailing should stop.
func (reader *ReaderImpl) readNewBytes(fileName string, bytesCount int64) (bool, error) {
	stream, _, newStat, err := zOpenWithStat(fileName)
	if err != nil {
		log.Debugf("Failed to open file %s for re-reading while tailing: %s", fileName, err.Error())
		return false, nil
	}

	if newStat != nil {
		reader.Lock()
		reader.lastStat = newStat
		reader.Unlock()
	}

	seekable, ok := stream.(io.ReadSeekCloser)
	if !ok {
		err = stream.Close()
		if err != nil {
			log.Debugf("Giving up on tailing, failed to close non-seekable stream from %s: %s", fileName, err.Error())
			return false, nil
		}
		log.Debugf("Giving up on tailing, file %s is not seekable", fileName)
		return false, nil
	}

	_, err = seekable.Seek(bytesCount, io.SeekStart)
	if err != nil {
		log.Debugf("Failed to seek in file %s while tailing: %s", fileName, err.Error())
		return false, nil
	}

	log.Tracef("File %s grew, reading more lines from byte %d...", fileName, bytesCount)

	reader.consumeLinesFromStream(seekable)
	err = seekable.Close()
	if err != nil {
		// This can lead to file handle leaks
		return false, fmt.Errorf("failed to close file %s after tailing: %w", fileName, err)
	}

	return true, nil
}

// headerChanged checks if the file's current starting bytes still match the
// header we have recorded so far. Returns true if they differ (file was
// rewritten) or if we are unsure due to errors.
func headerChanged(fileName string, readSoFar *readsummary.Summary) bool {
	headerBytes := readSoFar.Header()
	if len(headerBytes) == 0 {
		// We have no baseline to compare against (e.g., initially empty file).
		//
		// We safely return false here to allow it to be appended to (e.g. 0 ->
		// 123). If we returned true, an empty file that hasn't changed (0 -> 0)
		// would cause an infinite reload loop, and growing files would be
		// wastefully reloaded.
		return false
	}

	file, _, err := ZOpen(fileName)
	if err != nil {
		// Stat() succeeded just microseconds ago. If we can't open it now, the
		// file was likely deleted, rotated, or permissions changed. Safest to
		// reload.
		return true
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			log.Debugf("Failed to close file %s after checking matching bytes boundary: %s", fileName, closeErr.Error())
		}
	}()

	checkBuf := make([]byte, len(headerBytes))

	_, err = file.Read(checkBuf)
	if bytes.Equal(checkBuf, headerBytes) {
		return false
	}

	// Different bytes, check for errors
	if err != nil && err != io.EOF {
		// Something went wrong, avoid reloading since it could break things
		return false
	}

	// Bytes changed, no errors, so reload
	return true
}

// trailerChanged checks if the trailer we read last is still where we read it,
// just before the byte count. Returns true if it isn't (file was replaced by one
// starting out the same) or if we are unsure due to errors.
//
// Must not be called for compressed files, the byte count is not a file offset
// for those.
func trailerChanged(fileName string, readSoFar *readsummary.Summary) bool {
	bytesCount := readSoFar.Count()
	trailerBytes := readSoFar.Trailer()
	if len(trailerBytes) == 0 {
		// Nothing read so far, so anything is an append
		return false
	}

	file, err := os.Open(fileName)
	if err != nil {
		// Stat() succeeded just microseconds ago. If we can't open it now, the
		// file was likely deleted, rotated, or permissions changed. Safest to
		// reload.
		return true
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			log.Debugf("Failed to close file %s after checking trailing bytes: %s", fileName, closeErr.Error())
		}
	}()

	checkBuf := make([]byte, len(trailerBytes))
	trailerOffset := bytesCount - int64(len(trailerBytes))
	_, err = file.ReadAt(checkBuf, trailerOffset)
	if err != nil {
		// Either reading failed, or the file is shorter than what we have read.
		// The latter can happen since the caller compared the file size with
		// our last stat, not with bytesCount. Either way, we can't tell this
		// is an append, so reload.
		return true
	}

	return !bytes.Equal(checkBuf, trailerBytes)
}

// determineTailAction decides what to do about changes to the file we tail.
//
// readSoFar is read one getter at a time, so nothing may add to it while this
// runs. That holds as long as only the tailing goroutine adds to it, since
// this is called from that same goroutine.
func determineTailAction(
	fileName string,
	isCompressed bool,
	oldStat os.FileInfo,
	newStat os.FileInfo,
	statErr error,
	readSoFar *readsummary.Summary,
) tailAction {
	if statErr != nil {
		log.Debugf("Failed to stat file %s while tailing, giving up: %s", fileName, statErr.Error())
		return tailActionStop
	}

	if oldStat == nil {
		log.Debugf("Previous stat unknown for %s, stop tailing", fileName)
		return tailActionStop
	}

	if !sameFile(oldStat, newStat) {
		// Another file was moved into place. Even if it starts with the same
		// bytes as the old one, it is not an append.
		log.Debugf("File %s was replaced, reloading", fileName)
		return tailActionReload
	}

	oldSize := oldStat.Size()
	newSize := newStat.Size()

	if newSize < oldSize {
		log.Debugf("File %s shrunk, reloading", fileName)
		return tailActionReload
	}

	if newSize > oldSize {
		if isCompressed {
			log.Debugf("Compressed file %s grew, reloading", fileName)
			return tailActionReload
		}

		if headerChanged(fileName, readSoFar) {
			log.Debugf("File %s boundary bytes changed (likely rewritten) while growing, reloading", fileName)
			return tailActionReload
		}

		if trailerChanged(fileName, readSoFar) {
			log.Debugf("File %s trailing bytes changed (likely replaced) while growing, reloading", fileName)
			return tailActionReload
		}

		log.Debugf("File %s grew from %d to %d bytes (boundary OK), appending", fileName, oldSize, newSize)
		return tailActionAppend
	}

	// Invariant: File size unchanged

	if newStat.ModTime().After(oldStat.ModTime()) {
		log.Debugf("File %s got a new timestamp but size is unchanged, reloading", fileName)
		return tailActionReload
	}

	if headerChanged(fileName, readSoFar) {
		log.Debugf("File %s changed, reloading", fileName)
		return tailActionReload
	}

	// Invariant: File size unchanged and mod time unchanged

	log.Tracef("File %s unchanged at %d bytes, continue tailing", fileName, newSize)
	return tailActionContinue
}

// tailOnce performs one iteration of the file tailing check.
//
// Returns (shouldContinue, error): shouldContinue=false means tailing should stop.
func (reader *ReaderImpl) tailOnce() (bool, error) {
	reader.RLock()
	fileName := reader.FileName
	isCompressed := reader.IsCompressed
	oldStat := reader.lastStat
	reader.RUnlock()

	if fileName == nil {
		return false, nil
	}

	newStat, statErr := os.Stat(*fileName)
	action := determineTailAction(*fileName, isCompressed, oldStat, newStat, statErr, &reader.readSoFar)

	switch action {
	case tailActionStop:
		return false, nil
	case tailActionContinue:
		log.Tracef("File %s unchanged at %d bytes, continue tailing", *fileName, newStat.Size())
		return true, nil
	case tailActionReload:
		// Our clone will do the tailing from here
		log.Debugf("Requesting reload of %s", *fileName)
		reader.requestReload()
		return false, nil
	case tailActionAppend:
		return reader.readNewBytes(*fileName, reader.readSoFar.Count())
	default:
		return false, nil
	}
}

// tailFile polls the file for changes in a loop and updates the reader.
//
// Note: This starts executing ONLY after the initial parsing is completely
// finished (see `readStream`). Because initial parsing and tailing polling run
// sequentially on the same background goroutine, there is no concurrency (and
// thus no data races) between checking for appends and parsing original lines.
func (reader *ReaderImpl) tailFile() error {
	reader.RLock()
	fileName := reader.FileName
	reader.RUnlock()
	if fileName == nil {
		return nil
	}

	if !isSeekableFile(fileName) {
		log.Debugf("Giving up on tailing, %s is not seekable", *fileName)
		return nil
	}

	log.Debugf("Tailing file %s", *fileName)

	for !reader.closed.Load() {
		// NOTE: We could use something like
		// https://github.com/fsnotify/fsnotify instead of sleeping and polling
		// here.
		time.Sleep(tailPollInterval)

		shouldContinue, err := reader.tailOnce()
		if err != nil {
			return err
		}
		if !shouldContinue {
			return nil
		}
	}

	return nil
}

func isSeekableFile(fileName *string) bool {
	if fileName == nil {
		return false
	}

	file, err := os.Open(*fileName)
	if err != nil {
		return false
	}

	defer func() {
		err := file.Close()
		if err != nil {
			log.Debugf("Failed to close %s while checking seekability: %s", *fileName, err)
		}
	}()

	_, err = file.Seek(0, io.SeekCurrent)

	return err == nil
}
