package internal

import (
	"fmt"
	"sort"

	log "github.com/sirupsen/logrus"
	"github.com/walles/moor/v2/twin"
	"golang.org/x/exp/maps"
)

type PagerModeJumpToMark struct {
	pager *Pager
}

func (m PagerModeJumpToMark) drawFooter(_ string, _ string, _ string) {
	p := m.pager

	_, screenHeight := p.ScreenSize()
	height := int(screenHeight)

	pos := 0
	for _, token := range m.getMarkPrompt() {
		pos += p.screen.SetCell(pos, height-1, twin.NewStyledRune(token, twin.StyleDefault))
	}
}

func (m PagerModeJumpToMark) getMarkPrompt() string {
	// Special case having zero, one or multiple marks
	if len(m.pager.bookmarks) == 0 {
		markKey := keyForAction(m.pager.ModeBindings.Viewing, Mark)
		return fmt.Sprintf("No marks set, press '%s' to set one!", markKey)
	}

	if len(m.pager.bookmarks) == 1 {
		for key := range m.pager.bookmarks {
			return "Jump to your mark: " + string(key)
		}
	}

	// Multiple marks, list them
	marks := maps.Keys(m.pager.bookmarks)
	sort.Slice(marks, func(i, j int) bool {
		return marks[i] < marks[j]
	})

	prompt := "Jump to one of these marks: "
	for i, mark := range marks {
		if i > 0 {
			prompt += ", "
		}
		prompt += string(mark)
	}

	return prompt
}

func (m PagerModeJumpToMark) executeAction(action Action) {
	p := m.pager

	switch action {
	case Accept:
		if len(p.bookmarks) == 1 {
			for char, destination := range p.bookmarks {
				log.Debugf("Jumping to mark '%s'", string(char))
				p.scrollPosition = destination
			}
			p.mode = PagerModeViewing{pager: p}
		}
		return
	case Cancel:
		p.mode = PagerModeViewing{pager: p}
		return
	}

	// Try common actions
	if p.executeCommonAction(action) {
		return
	}

	// Action not handled
	log.Debugf("Unhandled jump-to-mark action: %v", action)
}

func (m PagerModeJumpToMark) onKey(key twin.KeyCode) {
	p := m.pager

	action, found := p.ModeBindings.JumpToMark.KeyCodeBindings[key]
	if found {
		m.executeAction(action)
		return
	}

	// No action bound, fall through to viewing mode
	log.Tracef("Unhandled jump-to-mark key event %v, treating as a viewing key event", key)
	p.mode = PagerModeViewing{pager: p}
	p.mode.onKey(key)
}

func (m PagerModeJumpToMark) onRune(char rune) {
	p := m.pager

	if len(p.bookmarks) == 0 {
		if action, found := p.ModeBindings.Viewing.RuneBindings[char]; found && action == Mark {
			p.mode = PagerModeMark(m)
			return
		}
	}

	destination, ok := p.bookmarks[char]
	if ok {
		log.Debugf("Jumping to mark '%s'", string(char))
		p.scrollPosition = destination
	}

	p.mode = PagerModeViewing(m)
}
