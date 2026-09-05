package internal

import (
	"fmt"

	log "github.com/sirupsen/logrus"
	"github.com/walles/twin"

	"github.com/walles/moor/v2/internal/linemetadata"
	"github.com/walles/moor/v2/internal/ptr"
	"github.com/walles/moor/v2/internal/search"
	"github.com/walles/moor/v2/internal/textstyles"
)

type PagerModeViewing struct {
	pager *Pager
}

func (m PagerModeViewing) drawFooter(filenameText string, statusText string, spinner string) {
	prefix := ""
	colonHelp := ""
	m.pager.readerLock.Lock()
	if len(m.pager.readers) > 1 {
		prefix = fmt.Sprintf("[%d/%d] ", m.pager.currentReader+1, len(m.pager.readers))
		colonHelp = "':' to switch, "
	}
	m.pager.readerLock.Unlock()

	searchHelp := "'/' to search"
	if !m.pager.search.Inactive() {
		searchHelp = "'n'/'p' to search next/previous"
	}
	helpText := "Press 'ESC' / 'q' to exit, " + colonHelp + searchHelp + ", '&' to filter, 'h' for help"

	if m.pager.isShowingHelp {
		helpText = "Press 'ESC' / 'q' to exit help, " + searchHelp
		prefix = ""
	}

	if m.pager.ShowStatusBar {
		if len(spinner) > 0 {
			spinner = "  " + spinner
		}
		m.pager.setFooter(prefix, filenameText, statusText+spinner, helpText)
	}
}

func (m PagerModeViewing) onKey(keyCode twin.KeyCode) {
	p := m.pager

	switch keyCode {
	case twin.KeyEscape:
		p.Quit()

	case twin.KeyUp:
		// Clipping is done in _Redraw()
		p.scrollPosition = p.scrollPosition.PreviousLine(1)
		p.handleScrolledUp()

	case twin.KeyDown, twin.KeyEnter:
		// Clipping is done in _Redraw()
		p.scrollPosition = p.scrollPosition.NextLine(1)
		p.handleScrolledDown()

	case twin.KeyRight:
		p.moveRight(p.SideScrollAmount)

	case twin.KeyLeft:
		p.moveRight(-p.SideScrollAmount)

	case twin.KeyAltRight:
		p.moveRight(1)

	case twin.KeyAltLeft:
		p.moveRight(-1)

	case twin.KeyHome:
		p.scrollPosition = newScrollPosition("Pager scroll position")
		p.handleScrolledUp()

	case twin.KeyEnd:
		p.scrollToEnd()

	case twin.KeyPgUp:
		p.scrollPosition = p.scrollPosition.PreviousLine(p.visibleHeight())
		p.handleScrolledUp()

	case twin.KeyPgDown:
		p.scrollPosition = p.scrollPosition.NextLine(p.visibleHeight())
		p.handleScrolledDown()

	default:
		log.Debugf("Unhandled key event %v", keyCode)
	}
}

func (m PagerModeViewing) onRune(char rune) {
	p := m.pager

	switch char {
	case 'q':
		p.Quit()

	case 'r':
		p.ReloadCurrentReader()

	case 'v':
		handleEditingRequest(p)

	case 'h':
		if p.isShowingHelp {
			break
		}

		p.preHelpState = &_PreHelpState{
			scrollPosition:      p.scrollPosition,
			leftColumnZeroBased: p.leftColumnZeroBased,
			targetLine:          p.TargetLine,
		}
		p.scrollPosition = newScrollPosition("Pager scroll position")
		p.leftColumnZeroBased = 0
		p.setTargetLine(nil)
		p.isShowingHelp = true

	case '=':
		p.ShowStatusBar = !p.ShowStatusBar

	// '\x10' = CTRL-p, should scroll up one line.
	// Ref: https://github.com/walles/moor/issues/107#issuecomment-1328354080
	case 'k', 'y', '\x10':
		// Clipping is done in _Redraw()
		p.scrollPosition = p.scrollPosition.PreviousLine(1)
		p.handleScrolledUp()

	// '\x0e' = CTRL-n, should scroll down one line.
	// Ref: https://github.com/walles/moor/issues/107#issuecomment-1328354080
	case 'j', 'e', '\x0e':
		// Clipping is done in _Redraw()
		p.scrollPosition = p.scrollPosition.NextLine(1)
		p.handleScrolledDown()

	case '<':
		p.scrollPosition = newScrollPosition("Pager scroll position")
		p.handleScrolledUp()

	case '>', 'G':
		p.scrollToEnd()

	// '\x06' = CTRL-f, should work like just 'f'.
	// Ref: https://github.com/walles/moor/issues/107
	case 'f', ' ', '\x06':
		p.scrollPosition = p.scrollPosition.NextLine(p.visibleHeight())
		p.handleScrolledDown()

	// '\x02' = CTRL-b, should work like just 'b'.
	// Ref: https://github.com/walles/moor/issues/107
	case 'b', '\x02':
		p.scrollPosition = p.scrollPosition.PreviousLine(p.visibleHeight())
		p.handleScrolledUp()

	// '\x15' = CTRL-u, should work like just 'u'.
	// Ref: https://github.com/walles/moor/issues/90
	case 'u', '\x15':
		p.scrollPosition = p.scrollPosition.PreviousLine(p.visibleHeight() / 2)
		p.handleScrolledUp()

	// '\x04' = CTRL-d, should work like just 'd'.
	// Ref: https://github.com/walles/moor/issues/90
	case 'd', '\x04':
		p.scrollPosition = p.scrollPosition.NextLine(p.visibleHeight() / 2)
		p.handleScrolledDown()

	case '/':
		p.mode = NewPagerModeSearch(p, SearchDirectionForward, p.scrollPosition)
		p.search.Clear()

		// Searchers want to scan the whole file, start reading as much as we can
		p.setTargetLine(ptr.To(linemetadata.IndexMax()))

	case '?':
		p.mode = NewPagerModeSearch(p, SearchDirectionBackward, p.scrollPosition)
		p.search.Clear()

		// Searchers want to scan the whole file, start reading as much as we can
		p.setTargetLine(ptr.To(linemetadata.IndexMax()))

	case '&':
		if !p.isShowingHelp {
			// Filtering the help text is not supported. Feel free to work on
			// that if you feel that's time well spent.
			p.mode = NewPagerModeFilter(p)
			p.search.Clear()
			p.filter = search.Search{}
		}

	case 'g':
		p.mode = NewPagerModeGotoLine(p)
		p.setTargetLine(nil)

	case ':':
		if len(p.readers) > 1 {
			p.mode = &PagerModeColonCommand{pager: p}
			p.setTargetLine(nil)
		} else {
			p.mode = &PagerModeInfo{Pager: p, Text: "Pass more files on the command line to be able to switch between them."}
		}

	// Should match the pagermode-not-found.go previous-search-hit bindings
	case 'n':
		p.scrollToNextSearchHit()

	// Should match the pagermode-not-found.go next-search-hit bindings
	case 'p', 'N':
		p.scrollToPreviousSearchHit()

	case 'm':
		p.mode = PagerModeMark{pager: p}
		p.setTargetLine(nil)

	case '\'':
		p.mode = PagerModeJumpToMark{pager: p}
		p.setTargetLine(nil)

	case 'w':
		p.WrapLongLines = !p.WrapLongLines
		if p.WrapLongLines {
			p.mode = &PagerModeInfo{Pager: p, Text: "Word wrapping enabled"}
		} else {
			p.mode = &PagerModeInfo{Pager: p, Text: "Word wrapping disabled"}
		}

	case '\x14': // CTRL-t
		p.cycleTabSize()

	case '\x01': // CTRL-a
		p.leftColumnZeroBased = 0
		if !p.showLineNumbers {
			// Line numbers not visible, turn them on if the user wants them.
			p.showLineNumbers = p.ShowLineNumbers
		}

	default:
		log.Debugf("Unhandled rune keypress '%s'/0x%08x", string(char), int32(char))
	}
}

func (p *Pager) cycleTabSize() {
	switch p.TabSize {
	case 8:
		p.TabSize = 4
	default:
		// We really want to toggle betwewen 4 and 8, but if we start out
		// somewhere else let's just go for 8. That's less' default tab size.
		p.TabSize = 8
	}
	textstyles.TabSize = p.TabSize

	p.mode = &PagerModeInfo{Pager: p, Text: fmt.Sprintf("Tab size set to %d", p.TabSize)}
}
