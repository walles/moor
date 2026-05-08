package search

import (
	"fmt"
	"sync"

	"github.com/walles/moor/v2/internal/linemetadata"
)

type SearchCache struct {
	mu sync.Mutex

	cache map[string]MatchRanges
	idx   int
}

func (sc *SearchCache) getKey(lineIdx linemetadata.Index, search string) string {
	return fmt.Sprintf("%v:%v:%v", search, lineIdx, sc.idx)
}

func (sc *SearchCache) SetReaderIdx(idx int) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	sc.idx = idx
}

func (sc *SearchCache) Get(lineIdx linemetadata.Index, search string) (MatchRanges, bool) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	res, pres := sc.cache[sc.getKey(lineIdx, search)]
	return res, pres
}

func (sc *SearchCache) Set(lineIdx linemetadata.Index, search string, value MatchRanges) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	sc.cache[sc.getKey(lineIdx, search)] = value
}

func NewSearchCache() *SearchCache {
	return &SearchCache{
		cache: make(map[string]MatchRanges),
	}
}
