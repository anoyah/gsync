package tui

import (
	"fmt"
	"strings"
)

// Every canvas row is self-contained (color and erase-to-end). Updating only
// changed rows avoids repainting the log pane when just the clock changes.
func frameDiff(previous, current string) string {
	if previous == current {
		return ""
	}
	if previous == "" {
		return current
	}
	a, b := strings.Split(previous, "\r\n"), strings.Split(current, "\r\n")
	if len(a) != len(b) {
		return current
	}
	var update strings.Builder
	for row, line := range b {
		if a[row] == line {
			continue
		}
		if row == 0 {
			line = strings.TrimPrefix(line, "\x1b[H")
		}
		fmt.Fprintf(&update, "\x1b[%d;1H%s", row+1, line)
	}
	if update.Len() >= len(current) {
		return current
	}
	return update.String()
}
