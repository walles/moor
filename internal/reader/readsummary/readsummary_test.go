package readsummary

import (
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

func TestEmpty(t *testing.T) {
	var s Summary

	assert.Equal(t, s.Count(), int64(0))
	assert.Equal(t, len(s.Header()), 0)
	assert.Equal(t, len(s.Trailer()), 0)
	assert.Equal(t, s.EndedWithNewline(), false)
}

func TestAdd(t *testing.T) {
	var s Summary

	s.Add([]byte("Hello\n"))
	assert.Equal(t, s.Count(), int64(6))
	assert.Equal(t, string(s.Header()), "Hello\n")
	assert.Equal(t, s.EndedWithNewline(), true)

	s.Add([]byte("World"))
	assert.Equal(t, s.Count(), int64(11))
	assert.Equal(t, string(s.Header()), "Hello\nWorld")
	assert.Equal(t, string(s.Trailer()), "Hello\nWorld")
	assert.Equal(t, s.EndedWithNewline(), false)
}

// The header should keep growing across chunks until it is full
func TestHeaderFillsUpAcrossChunks(t *testing.T) {
	var s Summary

	first := strings.Repeat("a", headerCapacity-1)
	s.Add([]byte(first))
	s.Add([]byte("bc"))

	assert.Equal(t, s.Count(), int64(headerCapacity+1))
	assert.Equal(t, string(s.Header()), first+"b")

	// With the header full, more chunks should leave it alone
	s.Add([]byte("de"))
	assert.Equal(t, s.Count(), int64(headerCapacity+3))
	assert.Equal(t, string(s.Header()), first+"b")
}

// The trailer should keep only the last bytes, across chunks
func TestTrailerKeepsTheLastBytes(t *testing.T) {
	var s Summary

	s.Add([]byte("ab"))
	middle := strings.Repeat("m", trailerCapacity-1)
	s.Add([]byte(middle))
	assert.Equal(t, string(s.Trailer()), "b"+middle)

	// One more byte should push the oldest one out
	s.Add([]byte("z"))
	assert.Equal(t, string(s.Trailer()), middle+"z")

	// A chunk larger than the trailer should replace it
	last := strings.Repeat("l", trailerCapacity+1)
	s.Add([]byte(last))
	assert.Equal(t, string(s.Trailer()), last[1:])
}

func TestEmptyChunkChangesNothing(t *testing.T) {
	var s Summary

	s.Add([]byte("Hello\n"))
	s.Add([]byte{})

	assert.Equal(t, s.Count(), int64(6))
	assert.Equal(t, s.EndedWithNewline(), true)
}

func TestHeaderIsACopy(t *testing.T) {
	var s Summary
	s.Add([]byte("Hello"))

	header := s.Header()
	header[0] = 'J'

	assert.Equal(t, string(s.Header()), "Hello")
}

func TestReset(t *testing.T) {
	var s Summary
	s.Add([]byte("Hello\n"))

	s.Reset()

	assert.Equal(t, s.Count(), int64(0))
	assert.Equal(t, len(s.Header()), 0)
	assert.Equal(t, len(s.Trailer()), 0)
	assert.Equal(t, s.EndedWithNewline(), false)

	s.Add([]byte("World"))
	assert.Equal(t, string(s.Header()), "World")
	assert.Equal(t, string(s.Trailer()), "World")
}

// Reading large files calls Add() for every chunk, so once the header and the
// trailer are full, it shouldn't allocate anything
func TestFullAddDoesNotAllocate(t *testing.T) {
	var s Summary
	bigChunk := []byte(strings.Repeat("a", 16*1024))
	smallChunk := []byte("small")
	s.Add(bigChunk)

	allocs := testing.AllocsPerRun(100, func() {
		s.Add(bigChunk)
		s.Add(smallChunk)
	})
	assert.Equal(t, allocs, 0.0)
}
