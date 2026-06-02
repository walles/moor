package internal

import (
	log "github.com/sirupsen/logrus"
	"github.com/walles/moor/v2/twin"
)

type SearchDirection bool

const (
	SearchDirectionForward  SearchDirection = false
	SearchDirectionBackward SearchDirection = true
)

type PagerModeSearch struct {
	pager                 *Pager
	initialScrollPosition scrollPosition // Pager position before search started
	direction             SearchDirection
	inputBox              *InputBox
	searchHistoryIndex    int
	userEditedText        string
}

func NewPagerModeSearch(p *Pager, direction SearchDirection, initialScrollPosition scrollPosition) *PagerModeSearch {
	m := &PagerModeSearch{
		pager:                 p,
		initialScrollPosition: initialScrollPosition,
		direction:             direction,
		searchHistoryIndex:    len(p.searchHistory.entries), // Past the end
	}
	m.inputBox = &InputBox{
		accept: INPUTBOX_ACCEPT_ALL,
		onTextChanged: func(text string) {
			m.pager.search.For(text)

			switch m.direction {
			case SearchDirectionBackward:
				m.pager.scrollToSearchHitsBackwards()
			case SearchDirectionForward:
				m.pager.scrollToSearchHits()
			}
		},
	}
	return m
}

func (m PagerModeSearch) drawFooter(_ string, _ string, _ []twin.StyledRune) {
	prompt := "Search: "
	if m.direction == SearchDirectionBackward {
		prompt = "Search backwards: "
	}
	m.inputBox.draw(m.pager.screen, "Type to search, 'ENTER' submits, 'ESC' cancels, '↑↓' navigate history", prompt)
}

func (m *PagerModeSearch) moveSearchHistoryIndex(delta int) {
	if len(m.pager.searchHistory.entries) == 0 {
		return
	}

	m.searchHistoryIndex += delta
	if m.searchHistoryIndex < 0 {
		m.searchHistoryIndex = 0
	}
	if m.searchHistoryIndex > len(m.pager.searchHistory.entries) {
		m.searchHistoryIndex = len(m.pager.searchHistory.entries) // Beyond the end of the history
	}

	if m.searchHistoryIndex == len(m.pager.searchHistory.entries) {
		// Reset to whatever the user typed last
		m.inputBox.setText(m.userEditedText)
	} else {
		// Get the history entry
		m.inputBox.setText(m.pager.searchHistory.entries[m.searchHistoryIndex])
	}
}

// Exit search mode, skip back to where we started
func (m *PagerModeSearch) abort() {
	m.pager.searchHistory.addEntry(m.inputBox.text)
	m.pager.mode = PagerModeViewing{pager: m.pager}
	m.pager.scrollPosition = m.initialScrollPosition
	m.pager.setTargetLine(nil) // Viewing doesn't need all lines
}

func (m *PagerModeSearch) onKey(key twin.KeyCode) {
	if m.inputBox.handleKey(key) {
		m.searchHistoryIndex = len(m.pager.searchHistory.entries) // Reset history index when user types
		m.userEditedText = m.inputBox.text
		return
	}

	switch key {
	case twin.KeyEnter:
		m.pager.searchHistory.addEntry(m.inputBox.text)
		m.pager.mode = PagerModeViewing{pager: m.pager}
		m.pager.setTargetLine(nil) // Viewing doesn't need all lines

	case twin.KeyEscape:
		m.abort()

	case twin.KeyPgUp, twin.KeyPgDown:
		m.pager.searchHistory.addEntry(m.inputBox.text)
		m.pager.mode = PagerModeViewing{pager: m.pager}
		m.pager.mode.onKey(key)
		m.pager.setTargetLine(nil) // Viewing doesn't need all lines

	case twin.KeyUp:
		m.moveSearchHistoryIndex(-1)

	case twin.KeyDown:
		m.moveSearchHistoryIndex(1)

	default:
		log.Debugf("Unhandled search key event %v", key)
	}
}

func (m *PagerModeSearch) onRune(char rune) {
	// Handle ctrl-c
	if char == '\x03' {
		m.abort()
		return
	}

	m.searchHistoryIndex = len(m.pager.searchHistory.entries) // Reset history index when user types
	m.inputBox.handleRune(char)
	m.userEditedText = m.inputBox.text
}
