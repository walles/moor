package internal

import (
	"os"
	"testing"

	"github.com/walles/moor/v2/internal/reader"
	"github.com/walles/moor/v2/twin"
	"gotest.tools/v3/assert"
)

func TestErrUnlessExecutable_yes(t *testing.T) {
	// Find our own executable
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	// Check that it's executable
	err = errUnlessExecutable(executable)
	if err != nil {
		t.Fatal(err)
	}
}

func TestErrUnlessExecutable_no(t *testing.T) {
	textFile := "pagermode-viewing_test.go"
	if _, err := os.Stat(textFile); os.IsNotExist(err) {
		t.Fatal("Test setup failed, text file not found: " + textFile)
	}

	err := errUnlessExecutable(textFile)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
}

func TestViewingFooter_WithSpinner(t *testing.T) {
	r := reader.NewFromTextForTesting("", "text")

	pager := NewPager(r)
	pager.ShowStatusBar = true

	// Attach a fake screen large enough to render the footer
	screen := twin.NewFakeScreen(80, 10)
	pager.screen = screen

	// Drive the footer rendering directly via PagerModeViewing
	mode := PagerModeViewing{pager: pager}
	spinner := "<->"
	mode.drawFooter("", "1 line  100%", spinner)

	footer := rowToString(screen.GetRow(9))

	// Quotes are stripped in rendering; expect plain keys
	expectedHelp := "Press escape to exit, / to search, & to filter, h for help"
	expected := "1 line  100%  " + spinner + "  " + expectedHelp

	assert.Equal(t, expected, footer)
}
