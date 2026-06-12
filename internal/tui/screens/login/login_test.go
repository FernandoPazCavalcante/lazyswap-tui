package login

import (
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/crypto"
	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/wallet"
)

func newTestDAO(t *testing.T) *wallet.DAO {
	t.Helper()
	dao, err := wallet.OpenAt(filepath.Join(t.TempDir(), "wallets.db"))
	if err != nil {
		t.Fatalf("OpenAt: %v", err)
	}
	t.Cleanup(func() { _ = dao.Close() })
	return dao
}

func typeAndEnter(m Model, text string) (Model, tea.Cmd) {
	m.input.SetValue(text)
	return m.Update(tea.KeyMsg{Type: tea.KeyEnter})
}

func TestNewFreshDB(t *testing.T) {
	dao := newTestDAO(t)
	m, err := New(dao)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if !m.IsFirstAccess() {
		t.Fatalf("fresh DB should be first access")
	}
	if m.AwaitingConfirm() {
		t.Fatalf("should not be awaiting confirm yet")
	}
}

func TestSubmitInvalidPasswordKeepsFirstStep(t *testing.T) {
	dao := newTestDAO(t)
	m, _ := New(dao)

	m, cmd := typeAndEnter(m, "short")
	if cmd != nil {
		t.Fatalf("invalid password should not start derive cmd")
	}
	if m.AwaitingConfirm() {
		t.Fatalf("invalid password should not advance to confirm")
	}
	if m.ErrMsg() == "" {
		t.Fatalf("expected an error message")
	}
}

func TestSubmitValidPasswordAdvancesToConfirm(t *testing.T) {
	dao := newTestDAO(t)
	m, _ := New(dao)

	m, cmd := typeAndEnter(m, "ValidPass1")
	if cmd != nil {
		t.Fatalf("first valid password should not start derive cmd")
	}
	if !m.AwaitingConfirm() {
		t.Fatalf("expected to be in confirm step")
	}
	if m.ErrMsg() != "" {
		t.Fatalf("unexpected error: %s", m.ErrMsg())
	}
}

func TestConfirmMismatchResetsState(t *testing.T) {
	dao := newTestDAO(t)
	m, _ := New(dao)

	m, _ = typeAndEnter(m, "ValidPass1")
	m, _ = typeAndEnter(m, "DifferentPw2")

	if m.AwaitingConfirm() {
		t.Fatalf("mismatch should reset awaitingConfirm")
	}
	if m.ErrMsg() == "" {
		t.Fatalf("expected mismatch error")
	}
}

func TestEscapeDuringConfirmResets(t *testing.T) {
	dao := newTestDAO(t)
	m, _ := New(dao)

	m, _ = typeAndEnter(m, "ValidPass1")
	if !m.AwaitingConfirm() {
		t.Fatalf("precondition: should be in confirm")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.AwaitingConfirm() {
		t.Fatalf("escape should reset confirm step")
	}
}

func TestConfirmMatchEmitsSuccessCmd(t *testing.T) {
	dao := newTestDAO(t)
	m, _ := New(dao)

	m, _ = typeAndEnter(m, "ValidPass1")
	m, cmd := typeAndEnter(m, "ValidPass1")

	if cmd == nil {
		t.Fatalf("expected derive cmd on match")
	}

	// Run the cmd synchronously and unwrap the resulting message.
	msg := cmd()
	internal, ok := msg.(loginInternalSuccessMsg)
	if !ok {
		t.Fatalf("expected loginInternalSuccessMsg, got %T (%+v)", msg, msg)
	}
	if internal.svc == nil {
		t.Fatalf("crypto service is nil")
	}

	// DB should now be initialised.
	ok, err := dao.IsEncryptionInitialised()
	if err != nil || !ok {
		t.Fatalf("DB should be initialised after success: ok=%v err=%v", ok, err)
	}

	// Feed the internal message back and verify the public LoginSuccessMsg fires.
	m, cmd = m.Update(internal)
	if cmd == nil {
		t.Fatalf("expected re-emit cmd")
	}
	pub, ok := cmd().(LoginSuccessMsg)
	if !ok {
		t.Fatalf("expected LoginSuccessMsg, got %T", cmd())
	}
	if pub.Service == nil {
		t.Fatalf("public msg carries nil service")
	}
}

func TestSubsequentLoginCorrectPassword(t *testing.T) {
	dao := newTestDAO(t)

	// Initialise the DB once via the first-access flow.
	m, _ := New(dao)
	m, _ = typeAndEnter(m, "ValidPass1")
	_, cmd := typeAndEnter(m, "ValidPass1")
	cmd() // executes the persist-and-sign block

	// Re-open: now in subsequent-login mode.
	m2, err := New(dao)
	if err != nil {
		t.Fatalf("New (second): %v", err)
	}
	if m2.IsFirstAccess() {
		t.Fatalf("second access should not be first-access")
	}

	_, cmd = typeAndEnter(m2, "ValidPass1")
	if cmd == nil {
		t.Fatalf("expected verify cmd")
	}
	msg := cmd()
	if _, ok := msg.(loginInternalSuccessMsg); !ok {
		t.Fatalf("expected login success, got %T (%+v)", msg, msg)
	}
}

func TestSubsequentLoginWrongPassword(t *testing.T) {
	dao := newTestDAO(t)
	m, _ := New(dao)
	m, _ = typeAndEnter(m, "ValidPass1")
	_, cmd := typeAndEnter(m, "ValidPass1")
	cmd()

	m2, _ := New(dao)
	_, cmd = typeAndEnter(m2, "WrongPass9")
	msg := cmd()
	errMsg, ok := msg.(loginErrMsg)
	if !ok {
		t.Fatalf("expected loginErrMsg, got %T", msg)
	}
	if errMsg.err == nil {
		t.Fatalf("error is nil")
	}
}

// guard: the sentinel constant matches the exported crypto value used by the
// derive command. Catches accidental divergence.
func TestSentinelConstant(t *testing.T) {
	if crypto.SentinelPlain != "lazyswap-v1-ok" {
		t.Fatalf("sentinel drift: %q", crypto.SentinelPlain)
	}
}
