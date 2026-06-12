// Package login implements the password-entry screen as a Bubble Tea model.
//
// Lifecycle:
//
//	First access  : enter password → confirm → derive key → persist salt + sentinel
//	Subsequent    : enter password → derive key → decrypt sentinel → verify
//
// Emits a LoginSuccessMsg carrying *crypto.Service when the user authenticates.
package login

import (
	"encoding/hex"
	"errors"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/crypto"
	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/tui/theme"
	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/wallet"
)

// LoginSuccessMsg is emitted to the parent when authentication completes.
type LoginSuccessMsg struct {
	Service *crypto.Service
}

// internal messages emitted by the derive/verify commands.
type (
	loginInternalSuccessMsg struct{ svc *crypto.Service }
	loginErrMsg             struct{ err error }
)

// Model owns the login screen state.
type Model struct {
	dao             *wallet.DAO
	input           textinput.Model
	isFirstAccess   bool
	awaitingConfirm bool
	firstEntry      string
	errMsg          string
	hint            string
	busy            bool
	width, height   int
}

// New constructs a login model and queries the DAO for first-access status.
func New(dao *wallet.DAO) (Model, error) {
	initialised, err := dao.IsEncryptionInitialised()
	if err != nil {
		return Model{}, err
	}

	ti := textinput.New()
	ti.EchoMode = textinput.EchoPassword
	ti.EchoCharacter = '•'
	ti.CharLimit = 128
	ti.Width = 32
	ti.Prompt = ""
	ti.Focus()

	m := Model{
		dao:           dao,
		input:         ti,
		isFirstAccess: !initialised,
	}
	m.hint = m.defaultHint()
	return m, nil
}

func (m Model) Init() tea.Cmd { return textinput.Blink }

// IsFirstAccess is exported for callers that need to render different
// surrounding chrome (e.g. a "Create a password" banner).
func (m Model) IsFirstAccess() bool { return m.isFirstAccess }

// AwaitingConfirm reports whether the user has entered the first password and
// is on the confirmation step.
func (m Model) AwaitingConfirm() bool { return m.awaitingConfirm }

// ErrMsg is the last user-facing error string, or "".
func (m Model) ErrMsg() string { return m.errMsg }

// Update processes a single message.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case loginInternalSuccessMsg:
		// Re-emit as the public message for the parent to consume.
		svc := msg.svc
		return m, func() tea.Msg { return LoginSuccessMsg{Service: svc} }

	case loginErrMsg:
		m.busy = false
		m.errMsg = msg.err.Error()
		m.input.SetValue("")
		return m, nil

	case tea.KeyMsg:
		if m.busy {
			return m, nil
		}
		switch msg.Type {
		case tea.KeyEnter:
			return m.submit()
		case tea.KeyEsc:
			if m.isFirstAccess && m.awaitingConfirm {
				m.resetConfirm()
				return m, nil
			}
		}
	}

	if m.busy {
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.errMsg = "" // clear stale error when user resumes typing
	return m, cmd
}

// View renders the centered login layout.
func (m Model) View() string {
	disclaimer := renderBetaDisclaimer()

	box := theme.Border(true).
		Width(36).
		Padding(0, 1).
		Render(m.input.View())

	rows := []string{
		theme.RenderLogo(),
		"",
		disclaimer,
		"",
		box,
		theme.Dim().Render(m.hint),
	}
	if m.errMsg != "" {
		rows = append(rows, theme.Error().Render(m.errMsg))
	}
	if m.busy {
		rows = append(rows, theme.Dim().Render("Deriving key..."))
	}
	rows = append(rows, "", theme.Dim().Render("enter: submit  ·  ctrl+c: quit"))

	content := lipgloss.JoinVertical(lipgloss.Center, rows...)
	if m.width == 0 || m.height == 0 {
		return content
	}
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

// renderBetaDisclaimer reproduces the OpenTUI beta box from
// src/tui/screens/login-screen.ts: rounded red border, dim-yellow background,
// width 58, centered four-line message.
func renderBetaDisclaimer() string {
	heading := theme.Error().Bold(true).Render("WARNING — BETA SOFTWARE")
	line1 := theme.Dim().Render("This software is in beta and may contain bugs.")
	line2 := theme.Error().Render("Do NOT use wallets holding real funds.")
	line3 := theme.Dim().Render("Use only for testing with small amounts.")

	body := lipgloss.JoinVertical(lipgloss.Center, heading, "", line1, line2, line3)

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Red).
		// Background(theme.NearBlackYellowSel).
		Padding(1, 2).
		Width(58).
		Align(lipgloss.Center).
		Render(body)
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func (m *Model) resetConfirm() {
	m.awaitingConfirm = false
	m.firstEntry = ""
	m.hint = m.defaultHint()
	m.input.SetValue("")
}

func (m Model) defaultHint() string {
	if m.isFirstAccess {
		return "Create a password  (8+ chars, A-Z a-z 0-9)"
	}
	return "Enter password"
}

func (m Model) submit() (Model, tea.Cmd) {
	pw := m.input.Value()
	m.input.SetValue("")
	m.errMsg = ""

	if m.isFirstAccess && !m.awaitingConfirm {
		if errStr := ValidatePassword(pw); errStr != "" {
			m.errMsg = errStr
			return m, nil
		}
		m.firstEntry = pw
		m.awaitingConfirm = true
		m.hint = "Confirm password"
		return m, nil
	}

	if m.isFirstAccess && m.awaitingConfirm {
		if pw != m.firstEntry {
			m.resetConfirm()
			m.errMsg = "Passwords do not match. Please try again."
			return m, nil
		}
		m.busy = true
		return m, deriveAndInitCmd(m.dao, pw)
	}

	// Subsequent login.
	m.busy = true
	return m, deriveAndVerifyCmd(m.dao, pw)
}

func deriveAndInitCmd(dao *wallet.DAO, pw string) tea.Cmd {
	return func() tea.Msg {
		key, salt, err := crypto.DeriveKey(pw, nil)
		if err != nil {
			return loginErrMsg{err}
		}
		svc, err := crypto.New(key)
		if err != nil {
			return loginErrMsg{err}
		}
		env, err := svc.Encrypt(crypto.SentinelPlain)
		if err != nil {
			return loginErrMsg{err}
		}
		if err := dao.SetSalt(hex.EncodeToString(salt)); err != nil {
			return loginErrMsg{err}
		}
		if err := dao.SetSentinel(env); err != nil {
			return loginErrMsg{err}
		}
		return loginInternalSuccessMsg{svc}
	}
}

func deriveAndVerifyCmd(dao *wallet.DAO, pw string) tea.Cmd {
	return func() tea.Msg {
		saltHex, ok, err := dao.GetSalt()
		if err != nil {
			return loginErrMsg{err}
		}
		if !ok {
			return loginErrMsg{errors.New("encryption salt missing — DB not initialised")}
		}
		salt, err := hex.DecodeString(saltHex)
		if err != nil {
			return loginErrMsg{err}
		}
		key, _, err := crypto.DeriveKey(pw, salt)
		if err != nil {
			return loginErrMsg{err}
		}
		svc, err := crypto.New(key)
		if err != nil {
			return loginErrMsg{err}
		}
		sentinel, ok, err := dao.GetSentinel()
		if err != nil {
			return loginErrMsg{err}
		}
		if !ok {
			return loginErrMsg{errors.New("password sentinel missing — DB not initialised")}
		}
		pt, err := svc.Decrypt(sentinel)
		if err != nil || pt != crypto.SentinelPlain {
			return loginErrMsg{errors.New("Invalid password. Try again.")}
		}
		return loginInternalSuccessMsg{svc}
	}
}
