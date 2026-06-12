// Package rightpanel provides the tab bar shared by the right-hand "Info"
// panel. Mirrors src/tui/panels/right-panel.ts buildLabel: it renders the list
// of enabled tabs and brackets whichever one is active.
package rightpanel

import (
	"fmt"
	"strings"
)

// Tab describes one selectable tab in the right-hand Info panel.
type Tab struct {
	Num   int    // the number key that selects it (1, 4, 5)
	Label string // human label, e.g. "Tokens"
}

// Header renders the tab bar as a single line, bracketing the active tab.
// e.g. " [1:Tokens]  4:Settings   5:Swap BTC "
func Header(active int, tabs []Tab) string {
	parts := make([]string, 0, len(tabs))
	for _, t := range tabs {
		seg := fmt.Sprintf("%d:%s", t.Num, t.Label)
		if t.Num == active {
			parts = append(parts, "["+seg+"]")
		} else {
			parts = append(parts, " "+seg+" ")
		}
	}
	return " " + strings.Join(parts, " ") + " "
}
