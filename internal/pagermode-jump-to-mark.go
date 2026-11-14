package internal

import (
	"fmt"
	"maps"
	"slices"

	"github.com/walles/twin"
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
		pos += p.screen.SetCell(pos, height-1, twin.StyledRune{Rune: token, Style: twin.StyleDefault})
	}
}

func (m PagerModeJumpToMark) getMarkPrompt() string {
	// Special case having zero, one or multiple marks
	if len(m.pager.bookmarks) == 0 {
		if m.pager.isShowingHelp {
			// Marks point into files, there is nothing to mark in help
			return "No marks set"
		}
		return "No marks set, press 'm' to set one!"
	}

	if len(m.pager.bookmarks) == 1 {
		for key := range m.pager.bookmarks {
			return "Jump to your mark: " + string(key)
		}
	}

	// Multiple marks, list them
	marks := slices.Sorted(maps.Keys(m.pager.bookmarks))

	prompt := "Jump to one of these marks: "
	for i, mark := range marks {
		if i > 0 {
			prompt += ", "
		}
		prompt += string(mark)
	}

	return prompt
}

func (m PagerModeJumpToMark) onKey(key twin.KeyCode) {
	p := m.pager

	switch key {
	case twin.KeyEnter, twin.KeyEscape:
		// Never mind I
		p.mode = PagerModeViewing{pager: p}

	default:
		// Never mind II
		p.mode = PagerModeViewing{pager: p}
		p.mode.onKey(key)
	}

	// Following paused while the prompt was open, catch up now rather than
	// when the next line arrives
	p.scrollTowardsTargetLine()
}

func (m PagerModeJumpToMark) onRune(char rune) {
	p := m.pager

	if len(p.bookmarks) == 0 {
		if char == 'm' && !p.isShowingHelp {
			// The prompt told the user to press 'm' to set a mark, see getMarkPrompt()
			p.mode = PagerModeMark(m)
			return
		}

		// Nothing to jump to, never mind
		p.mode = PagerModeViewing(m)
		p.mode.onRune(char)
		p.scrollTowardsTargetLine()
		return
	}

	// Before switching files, switchToFile() wants to be viewing
	p.mode = PagerModeViewing(m)

	destination, ok := p.bookmarks[char]
	if !ok {
		p.mode = &PagerModeInfo{Pager: p, Text: fmt.Sprintf("Mark %c is not set", char)}

		// Following paused while the prompt was open, catch up now rather
		// than when the next line arrives
		p.scrollTowardsTargetLine()
		return
	}

	if p.isShowingHelp {
		// Marks point into files, so go back to the files before jumping
		p.Quit()
	}

	p.switchToFile(destination.view)

	index, hidden := p.filteringReader.findLine(destination.line)
	if index == nil {
		p.scrollPosition = newScrollPosition("Jump to mark")
	} else {
		p.scrollPosition = NewScrollPositionFromIndex(*index, "Jump to mark")
	}
	p.setTargetLine(nil)

	if hidden {
		// We are now at the closest shown line above the mark, or at the top
		// if there is none
		p.mode = &PagerModeInfo{Pager: p, Text: fmt.Sprintf("Mark %c is hidden by the filter", char)}
	}
}
