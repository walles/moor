package internal

import (
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
		pos += p.screen.SetCell(pos, height-1, twin.NewStyledRune(token, twin.StyleDefault))
	}
}

func (m PagerModeJumpToMark) getMarkPrompt() string {
	// Special case having zero, one or multiple marks
	if len(m.pager.bookmarks) == 0 {
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
}

func (m PagerModeJumpToMark) onRune(char rune) {
	if len(m.pager.bookmarks) == 0 && char == 'm' {
		m.pager.mode = PagerModeMark(m)
		return
	}

	destination, ok := m.pager.bookmarks[char]
	if ok {
		m.pager.scrollPosition = destination
	}

	m.pager.mode = PagerModeViewing(m)
}
