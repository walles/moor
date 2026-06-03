package internal

import (
	"fmt"

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
	m.inputBox = NewInputBox(INPUTBOX_ACCEPT_ALL, p.ModeBindings.Input)
	m.inputBox.onTextChanged = func(text string) {
		m.pager.search.For(text)

		switch m.direction {
		case SearchDirectionBackward:
			m.pager.scrollToSearchHitsBackwards()
		case SearchDirectionForward:
			m.pager.scrollToSearchHits()
		}
	}
	return m
}

func (m PagerModeSearch) drawFooter(_ string, _ string, _ string) {
	prompt := "Search: "
	if m.direction == SearchDirectionBackward {
		prompt = "Search backwards: "
	}
	acceptKey := keyForAction(m.pager.ModeBindings.Search, Accept)
	cancelKey := keyForAction(m.pager.ModeBindings.Search, Cancel)
	histPrevKey := keyForAction(m.pager.ModeBindings.Search, HistoryPrevious)
	histNextKey := keyForAction(m.pager.ModeBindings.Search, HistoryNext)
	hint := fmt.Sprintf("Type to search, '%s' submits, '%s' cancels, '%s'/'%s' navigate history",
		acceptKey, cancelKey, histPrevKey, histNextKey)
	m.inputBox.draw(m.pager.screen, hint, prompt)
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

func (m *PagerModeSearch) executeAction(action Action) {
	// Handle search-specific actions first
	switch action {
	case Accept:
		m.pager.searchHistory.addEntry(m.inputBox.text)
		m.pager.mode = PagerModeViewing{pager: m.pager}
		m.pager.setTargetLine(nil) // Viewing doesn't need all lines
		return

	case Cancel:
		m.pager.searchHistory.addEntry(m.inputBox.text)
		m.pager.mode = PagerModeViewing{pager: m.pager}
		m.pager.scrollPosition = m.initialScrollPosition
		m.pager.setTargetLine(nil) // Viewing doesn't need all lines
		return

	case ScrollPageUp, ScrollPageDown:
		m.pager.searchHistory.addEntry(m.inputBox.text)
		m.pager.mode = PagerModeViewing{pager: m.pager}
		m.pager.executeCommonAction(action)
		m.pager.setTargetLine(nil) // Viewing doesn't need all lines
		return

	case HistoryPrevious:
		m.moveSearchHistoryIndex(-1)
		return

	case HistoryNext:
		m.moveSearchHistoryIndex(1)
		return
	}

	// Try common actions
	if m.pager.executeCommonAction(action) {
		return
	}

	// Action not handled
	log.Debugf("Unhandled search action %v", action)
}

func (m *PagerModeSearch) onKey(key twin.KeyCode) {
	// Check keybindings first (priority over InputBox)
	if action, found := m.pager.ModeBindings.Search.KeyCodeBindings[key]; found {
		m.executeAction(action)
		return
	}

	// Fall back to InputBox handling
	if m.inputBox.handleKey(key) {
		m.searchHistoryIndex = len(m.pager.searchHistory.entries) // Reset history index when user types
		m.userEditedText = m.inputBox.text
		return
	}

	log.Debugf("Unhandled search key event %v", key)
}

func (m *PagerModeSearch) onRune(char rune) {
	// Lookup action from keybindings
	if action, found := m.pager.ModeBindings.Search.RuneBindings[char]; found {
		m.executeAction(action)
		return
	}

	// Unbound rune - insert into search box
	m.searchHistoryIndex = len(m.pager.searchHistory.entries) // Reset history index when user types
	m.inputBox.handleRune(char)
	m.userEditedText = m.inputBox.text
}
