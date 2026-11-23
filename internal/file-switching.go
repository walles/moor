package internal

import (
	log "github.com/sirupsen/logrus"

	"github.com/walles/moor/v2/internal/linemetadata"
	"github.com/walles/moor/v2/internal/ptr"
)

func (p *Pager) previousFile() {
	p.switchToFile(p.currentView - 1)
}

func (p *Pager) nextFile() {
	p.switchToFile(p.currentView + 1)
}

func (p *Pager) firstFile() {
	p.switchToFile(0)
}

// Save the state of the current reader and restore the state of the new one.
//
// Must be called from the main loop goroutine, which is also the only one
// changing currentView. That's why we can read it without locking.
func (p *Pager) switchToFile(newIndex int) {
	newIndex = max(0, min(newIndex, len(p.views)-1))
	if newIndex == p.currentView {
		// Following paused while the colon prompt was open, catch up now
		// rather than when the next line arrives
		p.scrollTowardsTargetLine()
		return
	}

	// Save where we are. If we're on our way somewhere, like when following,
	// remember that rather than where we happen to be right now.
	leaving := &p.views[p.currentView]
	leaving.targetLine = linemetadata.IndexFromZeroBased(0) // For empty files
	if p.TargetLine != nil {
		leaving.targetLine = *p.TargetLine
	} else if index := p.lineIndex(); index != nil {
		leaving.targetLine = *index
	}
	leaving.horizontal = ptr.To(p.getHorizontalPosition())
	leaving.filter = p.filter

	p.readerLock.Lock()
	p.currentView = newIndex
	p.filteringReader.SetBackingReader(p.views[newIndex].reader)
	p.readerLock.Unlock()

	entering := p.views[newIndex]
	p.filter = entering.filter
	if entering.horizontal != nil {
		p.setHorizontalPosition(*entering.horizontal)
	} else {
		p.setHorizontalPosition(horizontalPosition{showLineNumbers: p.ShowLineNumbers})
	}
	p.scrollPosition = newScrollPosition("Pager file switch")
	p.setTargetLine(ptr.To(entering.targetLine))

	// Before drawing anything, or we'll draw the old file contents. After
	// setting the target line, so that the reload waits for the lines we're
	// going to show.
	p.reloadCurrentReaderIfWanted()

	// The readerSwitched notification below gets us here too, but not before
	// the main loop has drawn one frame of the top of the file
	p.scrollTowardsTargetLine()

	log.Infof("Switched to file index %d", p.currentView)

	select {
	case p.readerSwitched <- struct{}{}:
	default:
	}
}
