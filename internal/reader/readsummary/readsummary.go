// Package readsummary keeps track of what has been read from a file, to be
// able to tell appends apart from the file being replaced.
package readsummary

import (
	"slices"
	"sync"
)

// How many bytes to keep from each end of what we have read. Comparing these
// with the file tells replacement apart from appending when it grows.
const (
	headerCapacity  = 1024
	trailerCapacity = 1024
)

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

	// The last bytes read, up to trailerCapacity of them.
	trailer []byte

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

	// Keep only the last bytes in the trailer, in one buffer we never
	// reallocate. Start with the last bytes of the chunk...
	if s.trailer == nil {
		s.trailer = make([]byte, 0, trailerCapacity)
	}
	fitsInTrailer := min(trailerCapacity, len(chunk))
	newTrailerBytes := chunk[len(chunk)-fitsInTrailer:]

	// ... make room for them by moving the newest old bytes we keep down to
	// the start of the buffer...
	keep := min(len(s.trailer), trailerCapacity-len(newTrailerBytes))
	copy(s.trailer, s.trailer[len(s.trailer)-keep:])

	// ... and add the new bytes after them
	s.trailer = append(s.trailer[:keep], newTrailerBytes...)
}

// Reset forgets everything added so far.
func (s *Summary) Reset() {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.count = 0
	s.header = nil
	s.trailer = s.trailer[:0]
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

// Header returns a copy of the first bytes added, up to headerCapacity of
// them. Used to determine if the file was replaced or appended to when it
// grows.
func (s *Summary) Header() []byte {
	s.lock.Lock()
	defer s.lock.Unlock()

	return slices.Clone(s.header)
}

// Trailer returns a copy of the last bytes added, up to trailerCapacity of
// them. Used to determine if the file was replaced or appended to when it
// grows.
func (s *Summary) Trailer() []byte {
	s.lock.Lock()
	defer s.lock.Unlock()

	return slices.Clone(s.trailer)
}

// EndedWithNewline tells whether the last byte added was a newline.
func (s *Summary) EndedWithNewline() bool {
	s.lock.Lock()
	defer s.lock.Unlock()

	return s.endedWithNewline
}
