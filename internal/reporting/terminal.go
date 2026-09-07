package reporting

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/mattn/go-runewidth"
	"golang.org/x/term"
)

var getTerminalSize = term.GetSize

var ansiSequencePattern = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)

func (r *Reporter) redrawLocked() error {
	stage := r.ensureStageLocked()
	r.refreshTerminalDimensionsLocked()
	content := stage.render()
	var redraw strings.Builder
	if stage.rendered {
		previousRows := stage.renderedRows
		if stage.renderedWidth != r.terminalWidth || stage.renderedHeight != r.terminalHeight {
			previousRows = r.visibleRowsToCursor(stage.renderedContent)
		}
		if previousRows > 0 {
			fmt.Fprintf(&redraw, "\x1b[%dA", previousRows)
		}
		redraw.WriteString("\r\x1b[J")
	}
	redraw.WriteString(content)
	if err := r.writeLocked(redraw.String()); err != nil {
		return err
	}
	stage.renderedContent = content
	stage.renderedRows = r.visibleRowsToCursor(content)
	stage.renderedWidth = r.terminalWidth
	stage.renderedHeight = r.terminalHeight
	stage.rendered = true
	return nil
}

func (r *Reporter) refreshTerminalDimensionsLocked() {
	if descriptor, ok := r.output.(interface{ Fd() uintptr }); ok {
		width, height, err := getTerminalSize(int(descriptor.Fd()))
		if err == nil && width > 0 && height > 0 {
			r.terminalWidth = width
			r.terminalHeight = height
		}
	}
}

func (r *Reporter) visibleRowsToCursor(content string) int {
	rows := visualRowsToCursor(content, r.terminalWidth)
	if r.terminalHeight > 0 && rows >= r.terminalHeight {
		return r.terminalHeight - 1
	}
	return rows
}

func visualRowsToCursor(content string, width int) int {
	if content == "" || width <= 0 {
		return 0
	}

	content = ansiSequencePattern.ReplaceAllString(content, "")
	rows := 0
	columns := 0
	textStart := 0
	for index := 0; index < len(content); index++ {
		switch content[index] {
		case '\n':
			columns += runewidth.StringWidth(content[textStart:index])
			rows += wrappedRows(columns, width) + 1
			columns = 0
			textStart = index + 1
		case '\r':
			columns += runewidth.StringWidth(content[textStart:index])
			rows, columns = rows+wrappedRows(columns, width), 0
			textStart = index + 1
		case '\t':
			columns += runewidth.StringWidth(content[textStart:index])
			columns += 8 - columns%8
			textStart = index + 1
		}
	}
	columns += runewidth.StringWidth(content[textStart:])
	if !strings.HasSuffix(content, "\n") {
		rows += wrappedRows(columns, width)
	}
	return rows
}

func wrappedRows(columns, width int) int {
	return max(1, (columns+width-1)/width) - 1
}
