// Package importoverlay provides the modal mnemonic-import overlay.
//
// Mirrors src/tui/overlays/import-overlay.ts: full-screen textinput,
// ESC cancels, ENTER submits. Emits SubmitMsg with the trimmed phrase
// or CancelMsg on escape.
package importoverlay

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/tui/theme"
)

type SubmitMsg struct{ Phrase string }
type CancelMsg struct{}

type Model struct {
	input         textinput.Model
	errMsg        string
	width, height int
}

func New() Model {
	ti := textinput.New()
	ti.Placeholder = "twelve word mnemonic..."
	ti.CharLimit = 256
	ti.Width = 60
	ti.Prompt = "› "
	ti.Focus()
	return Model{input: ti}
}

func (m Model) Init() tea.Cmd { return textinput.Blink }

func (m *Model) SetSize(w, h int) { m.width, m.height = w, h }

// SetErr displays an inline error (used by the parent on import failure).
func (m *Model) SetErr(s string) { m.errMsg = s }

// Value returns the current text-input contents (mostly for tests).
func (m Model) Value() string { return m.input.Value() }

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEsc:
			return m, func() tea.Msg { return CancelMsg{} }
		case tea.KeyEnter:
			phrase := strings.TrimSpace(m.input.Value())
			return m, func() tea.Msg { return SubmitMsg{Phrase: phrase} }
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if m.input.Value() != "" {
		m.errMsg = "" // clear stale error when user resumes typing
	}
	return m, cmd
}

func (m Model) View() string {
	box := theme.Border(true).
		Width(64).
		Padding(1, 2).
		Render(lipgloss.JoinVertical(
			lipgloss.Left,
			theme.Text().Bold(true).Render("Import wallet"),
			"",
			theme.Dim().Render("Paste a 12 / 24 word BIP-39 mnemonic phrase."),
			"",
			m.input.View(),
		))

	rows := []string{box}
	if m.errMsg != "" {
		rows = append(rows, theme.Error().Render(m.errMsg))
	}
	rows = append(rows, theme.Dim().Render("enter: import  ·  esc: cancel"))

	content := lipgloss.JoinVertical(lipgloss.Center, rows...)
	if m.width == 0 || m.height == 0 {
		return content
	}
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}
