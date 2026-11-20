// Package readsummary keeps track of what has been read from a file, to be
// able to tell appends apart from the file being replaced.
package readsummary

import (
	"slices"
	"sync"
)

// Track the first N bytes read from the file, to be able to tell apart file
// replacement from appending when the file grows.
const headerCapacity = 1024

// Summary is what we know about the bytes read so far, across the initial read
// and all appends.
//
// It is safe for concurrent use. It must not be copied after first use.
type Summary struct {
	lock sync.Mutex

	// How many bytes have been read
	count int64

	// The first bytes read, up to headerCapacity of them.
	header []byte

	// Whether the last byte read was a newline
	endedWithNewline bool
}

// Add records bytes that have been read.
func (s *Summary) Add(chunk []byte) {
	if len(chunk) == 0 {
		return
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	s.count += int64(len(chunk))
	s.endedWithNewline = chunk[len(chunk)-1] == '\n'

	// Fill up the header if there's room left in it
	headerRoom := headerCapacity - len(s.header)
	fitsInHeader := min(headerRoom, len(chunk))
	s.header = append(s.header, chunk[:fitsInHeader]...)
}

// Reset forgets everything added so far.
func (s *Summary) Reset() {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.count = 0
	s.header = nil
	s.endedWithNewline = false
}

// Count is how many bytes have been added.
//
// Note: For compressed files, this is the DECOMPRESSED byte count. Do NOT
// compare this directly to os.FileInfo.Size() (which is the compressed size on
// disk), as they represent different metrics.
func (s *Summary) Count() int64 {
	s.lock.Lock()
	defer s.lock.Unlock()

	return s.count
}

// Header returns a copy of the first bytes added, up to 1024 of them. Used
// to determine if the file was replaced or appended to when it grows.
func (s *Summary) Header() []byte {
	s.lock.Lock()
	defer s.lock.Unlock()

	return slices.Clone(s.header)
}

// EndedWithNewline tells whether the last byte added was a newline.
func (s *Summary) EndedWithNewline() bool {
	s.lock.Lock()
	defer s.lock.Unlock()

	return s.endedWithNewline
}
