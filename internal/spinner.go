package internal

import (
	"time"

	"github.com/walles/moor/v2/twin"
)

const millisPerCycle = 5000

func getSpinnerFrame() []twin.StyledRune {
	// Zero to 255, inclusive
	progress := -time.Now().UnixMilli() * 256 / millisPerCycle % 256

	length := 3

	spinner := make([]twin.StyledRune, 0, length)
	for range length {
		level := uint8(progress)
		spinner = append(spinner, twin.NewStyledRune(' ',
			twin.StyleDefault.WithBackground(twin.NewColor24Bit(level, level, level))))
	}

	return spinner
}
