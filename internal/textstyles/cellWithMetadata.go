package textstyles

import (
	"slices"
	"unicode"

	"github.com/walles/twin"
)

// Like twin.StyledRune, but with additional metadata
type CellWithMetadata struct {
	Rune  rune
	Style twin.Style

	cachedWidth *int

	StartsSearchHit bool // True if this cell is the start of a search hit
	IsSearchHit     bool // True if this cell is part of a search hit
}

// Required for some tests to pass
func (r CellWithMetadata) Equal(b CellWithMetadata) bool {
	if r.Rune != b.Rune {
		return false
	}

	if !r.Style.Equal(b.Style) {
		return false
	}

	if r.IsSearchHit != b.IsSearchHit {
		return false
	}

	if r.StartsSearchHit != b.StartsSearchHit {
		return false
	}

	return true
}

func (r CellWithMetadata) ToStyledRune() twin.StyledRune {
	return twin.StyledRune{Rune: r.Rune, Style: r.Style}
}

func (r *CellWithMetadata) Width() int {
	if r.cachedWidth != nil {
		return *r.cachedWidth
	}

	// Cache it
	w := r.ToStyledRune().Width()
	r.cachedWidth = &w
	return w
}

type CellWithMetadataSlice []CellWithMetadata

func (runes CellWithMetadataSlice) Equal(other CellWithMetadataSlice) bool {
	if len(runes) != len(other) {
		return false
	}

	for i := range runes {
		if !runes[i].Equal(other[i]) {
			return false
		}
	}

	return true
}

// Returns a copy of the slice with leading whitespace removed
func (runes CellWithMetadataSlice) WithoutSpaceLeft() CellWithMetadataSlice {
	for i := range runes {
		cell := runes[i]
		if !unicode.IsSpace(cell.Rune) {
			return runes[i:]
		}

		// That was a space, keep looking
	}

	// All whitespace, return empty
	return CellWithMetadataSlice{}
}

// Returns a copy of the slice with trailing whitespace removed
func (runes CellWithMetadataSlice) WithoutSpaceRight() CellWithMetadataSlice {
	for i, cell := range slices.Backward(runes) {
		if !unicode.IsSpace(cell.Rune) {
			return runes[0 : i+1]
		}

		// That was a space, keep looking
	}

	// All whitespace, return empty
	return CellWithMetadataSlice{}
}

func (runes CellWithMetadataSlice) ContainsSearchHit() bool {
	for _, cell := range runes {
		if cell.IsSearchHit {
			return true
		}
	}

	return false
}
