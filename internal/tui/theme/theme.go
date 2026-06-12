// Package theme exposes the lipgloss color palette used across the TUI.
//
// Mirrors the OpenTUI theme.ts constants from the Bun reference:
//   YELLOW     = #FFFF00  primary text / focused borders
//   YELLOW_DIM = #787800  hints / unfocused borders
//   YELLOW_SEL = #1E1E00  selected row background
//   NEAR_BLACK = #080802  app background
//   RED        = #FF0000  destructive / warnings
package theme

import "github.com/charmbracelet/lipgloss"

var (
	Yellow    = lipgloss.Color("#FFFF00")
	YellowDim = lipgloss.Color("#787800")
	YellowSel = lipgloss.Color("#1E1E00")
	NearBlack = lipgloss.Color("#080802")
	Red       = lipgloss.Color("#FF0000")
)

// Border returns the standard rounded border in the focused/unfocused color.
func Border(focused bool) lipgloss.Style {
	c := YellowDim
	if focused {
		c = Yellow
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(c)
}

// Text returns a baseline yellow text style.
func Text() lipgloss.Style { return lipgloss.NewStyle().Foreground(Yellow) }

// Dim returns a dim text style.
func Dim() lipgloss.Style { return lipgloss.NewStyle().Foreground(YellowDim) }

// Error returns a red text style.
func Error() lipgloss.Style { return lipgloss.NewStyle().Foreground(Red) }
