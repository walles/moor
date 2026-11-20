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
	assert.Equal(t, s.EndedWithNewline(), false)

	s.Add([]byte("World"))
	assert.Equal(t, string(s.Header()), "World")
}
