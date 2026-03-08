//go:build !windows

package twin

import (
	"io"
	"os"
	"runtime/debug"
	"testing"
	"time"

	"gotest.tools/v3/assert"
)

// This test should replace TestInterruptableReader_blockedOnRead if or when
// Windows catches up with the shutdown implementation.
func TestInterruptableReader_blockedOnReadImmediate(t *testing.T) {
	// Make a pipe to read from and write to
	pipeReader, pipeWriter, err := os.Pipe()
	assert.NilError(t, err)

	// Make an interruptable reader
	testMe, err := newInterruptableReader(pipeReader)
	assert.NilError(t, err)
	assert.Assert(t, testMe != nil)

	// Start a thread that reads from the pipe
	type readResult struct {
		n   int
		err error
	}
	readResultChan := make(chan readResult)
	go func() {
		defer func() {
			panicHandler("TestInterruptableReader_blockedOnReadImmediate()", recover(), debug.Stack())
		}()

		buffer := make([]byte, 1)
		n, err := testMe.Read(buffer)
		readResultChan <- readResult{n, err}
	}()

	// Give the reader thread some time to start waiting
	time.Sleep(100 * time.Millisecond)

	// Interrupt the reader
	testMe.Interrupt()

	// Wait for the reader thread to finish
	result := <-readResultChan

	// Check the result
	assert.Equal(t, result.n, 0)
	assert.Equal(t, result.err, io.EOF)

	// Another read should return EOF immediately
	buffer := make([]byte, 1)
	n, err := testMe.Read(buffer)
	assert.Equal(t, err, io.EOF)
	assert.Equal(t, n, 0)

	// Even if there are bytes, the interrupted reader should still return EOF
	n, err = pipeWriter.Write([]byte{42})
	assert.NilError(t, err)
	assert.Equal(t, n, 1)

	n, err = testMe.Read(buffer)
	assert.Equal(t, err, io.EOF)
	assert.Equal(t, n, 0)
}

func TestInterruptableReader_wakeupDoesNotShutdown(t *testing.T) {
	pipeReader, pipeWriter, err := os.Pipe()
	assert.NilError(t, err)

	testMe, err := newInterruptableReader(pipeReader)
	assert.NilError(t, err)

	type readResult struct {
		n   int
		err error
	}

	// Background read once
	readResultChan := make(chan readResult)
	go func() {
		defer func() {
			panicHandler("TestInterruptableReader_wakeupDoesNotShutdown()", recover(), debug.Stack())
		}()

		buffer := make([]byte, 1)
		n, err := testMe.Read(buffer)
		readResultChan <- readResult{n, err}
	}()

	// Give the reader thread some time to start awaiting bytes
	time.Sleep(100 * time.Millisecond)

	// Wakeup the reader
	err = testMe.Wakeup()
	assert.NilError(t, err)

	// Since we poked it, the reader should now return an empty result
	result := <-readResultChan
	assert.Equal(t, result.n, 0)
	assert.NilError(t, result.err)

	// Write something...
	written, err := pipeWriter.Write([]byte{42})
	assert.NilError(t, err)
	assert.Equal(t, written, 1)

	// ... and we should be able to get it back from the reader
	buffer := make([]byte, 1)
	n, err := testMe.Read(buffer)
	assert.NilError(t, err)
	assert.Equal(t, n, 1)
	assert.Equal(t, buffer[0], byte(42))
}
