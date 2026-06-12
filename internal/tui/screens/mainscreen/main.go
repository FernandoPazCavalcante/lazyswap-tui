// Package mainscreen owns the post-login layout: wallet panel (left) +
// tokens panel (right), plus overlay routing for create / delete / import.
package mainscreen

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	qrcode "github.com/skip2/go-qrcode"

	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/applog"
	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/balance"
	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/chain"
	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/swap"
	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/tui/overlays/importoverlay"
	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/tui/overlays/swapoverlay"
	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/tui/panels/rightpanel"
	settingspanel "github.com/FernandoPazCavalcante/lazyswap-tui/internal/tui/panels/settings"
	swapbtcpanel "github.com/FernandoPazCavalcante/lazyswap-tui/internal/tui/panels/swapbtc"
	tokenspanel "github.com/FernandoPazCavalcante/lazyswap-tui/internal/tui/panels/tokens"
	walletpanel "github.com/FernandoPazCavalcante/lazyswap-tui/internal/tui/panels/wallet"
	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/tui/theme"
	walletpkg "github.com/FernandoPazCavalcante/lazyswap-tui/internal/wallet"
)

// DefaultSlippage is the slippage percentage applied to every quoted swap.
const DefaultSlippage = 0.5

type mode int

const (
	modeNormal mode = iota
	modeConfirmCreate
	modeConfirmDelete
	modeImport
	modeSwap
	modeWalletQR
)

type focused int

const (
	focusLeft focused = iota
	focusRight
)

// tab identifies a right-panel tab. Values match the number key that selects
// the tab and the labels in src/tui/panels/right-panel.ts.
type tab int

const (
	tabTokens   tab = 1
	tabSettings tab = 4
	tabSwapBTC  tab = 5
)

// Model owns the main-screen state machine.
type Model struct {
	svc     *walletpkg.Service
	balSvc  *balance.Service
	flowSvc *swap.Flow

	panel    walletpanel.Model
	tokens   tokenspanel.Model
	settings settingspanel.Model
	swapbtc  swapbtcpanel.Model
	imp      importoverlay.Model
	swap     swapoverlay.Model

	mode      mode
	focus     focused
	activeTab tab
	errMsg    string
	// notice is a transient confirmation (e.g. "address copied") shown in the
	// footer in place of an error. walletCopyStatus is the feedback line inside
	// the wallet QR overlay.
	notice           string
	walletCopyStatus string

	// chainKey + slippage are the authoritative swap parameters; the settings
	// tab edits them via messages, the swap flow consumes them.
	chainKey string
	slippage float64

	wallets []walletpkg.Wallet
	current *walletpkg.Wallet

	// Cache balances per wallet address so flipping wallets doesn't re-fetch.
	balanceCache map[string][]balance.TokenBalance

	width, height int
}

// New constructs a main-screen model. balSvc / flowSvc may be nil — when
// either is missing the corresponding feature is disabled but the panel
// remains usable (useful for tests). chainKey selects the active chain; pass
// an empty string to use chain.DefaultKey.
func New(svc *walletpkg.Service, balSvc *balance.Service, flowSvc *swap.Flow, chainKey string) Model {
	if chainKey == "" {
		chainKey = chain.DefaultKey
	}
	c := chain.Get(chainKey)
	return Model{
		svc:          svc,
		balSvc:       balSvc,
		flowSvc:      flowSvc,
		panel:        walletpanel.New(),
		tokens:       tokenspanel.New(),
		settings:     settingspanel.New(DefaultSlippage, chainKey, c.Name),
		swapbtc:      swapbtcpanel.New(),
		imp:          importoverlay.New(),
		focus:        focusLeft,
		activeTab:    tabTokens,
		chainKey:     chainKey,
		slippage:     DefaultSlippage,
		balanceCache: make(map[string][]balance.TokenBalance),
	}
}

// Init kicks off the initial wallet load.
func (m Model) Init() tea.Cmd { return refreshWalletsCmd(m.svc) }

// footerRows is the number of rows reserved below the panels: one for the
// optional error line + one for the status bar. Kept constant so panel sizes
// don't shift when an error appears.
const footerRows = 2

// tabHeaderRows is the single row reserved at the top of the right column for
// the tab bar (mirrors right-panel.ts tabHeader).
const tabHeaderRows = 1

// SetSize lays out the inner components against the outer terminal size.
func (m *Model) SetSize(w, h int) {
	m.width, m.height = w, h
	leftW := w / 3
	if leftW < 24 {
		leftW = 24
	}
	rightW := w - leftW
	if rightW < 1 {
		rightW = 1
	}
	bodyH := h - footerRows
	if bodyH < 1 {
		bodyH = 1
	}
	rightH := bodyH - tabHeaderRows
	if rightH < 1 {
		rightH = 1
	}
	m.panel.SetSize(leftW, bodyH)
	m.tokens.SetSize(rightW, rightH)
	m.settings.SetSize(rightW, rightH)
	m.swapbtc.SetSize(rightW, rightH)
	m.imp.SetSize(w, h)
	if m.mode == modeSwap {
		m.swap.SetSize(w, h)
	}
	m.applyFocusStyles()
}

func (m *Model) applyFocusStyles() {
	m.panel.SetFocused(m.focus == focusLeft)
	rightFocused := m.focus == focusRight
	m.tokens.SetFocused(rightFocused && m.activeTab == tabTokens)
	m.settings.SetFocused(rightFocused && m.activeTab == tabSettings)
	m.swapbtc.SetFocused(rightFocused && m.activeTab == tabSwapBTC)
}

// ─── Messages ────────────────────────────────────────────────────────────────

type walletsRefreshedMsg struct {
	wallets []walletpkg.Wallet
	err     error
}
type createdMsg struct {
	w   *walletpkg.Wallet
	err error
}
type importedMsg struct {
	w   *walletpkg.Wallet
	err error
}
type deletedMsg struct{ err error }
type balancesFetchedMsg struct {
	walletAddress string
	balances      []balance.TokenBalance
	err           error
}
type swapQuoteMsg struct {
	quote swap.FlowQuote
	err   error
}
type swapExecMsg struct{ result swap.FlowResult }

func refreshWalletsCmd(svc *walletpkg.Service) tea.Cmd {
	return func() tea.Msg {
		ws, err := svc.FetchAll()
		return walletsRefreshedMsg{wallets: ws, err: err}
	}
}
func createCmd(svc *walletpkg.Service) tea.Cmd {
	return func() tea.Msg {
		w, err := svc.Create()
		return createdMsg{w: w, err: err}
	}
}
func importCmd(svc *walletpkg.Service, phrase string) tea.Cmd {
	return func() tea.Msg {
		w, err := svc.Import(phrase)
		return importedMsg{w: w, err: err}
	}
}
func deleteCmd(svc *walletpkg.Service, id string) tea.Cmd {
	return func() tea.Msg {
		return deletedMsg{err: svc.Delete(id)}
	}
}
func quoteSwapCmd(f *swap.Flow, from, to swap.TokenInfo, usd, walletAddr string, slippage float64) tea.Cmd {
	return func() tea.Msg {
		if f == nil {
			return swapQuoteMsg{err: fmt.Errorf("swap service not configured")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		q, err := f.Quote(ctx, from, to, usd, slippage, walletAddr)
		return swapQuoteMsg{quote: q, err: err}
	}
}

func executeSwapCmd(f *swap.Flow, privateKey string, from, to swap.TokenInfo, usd string, slippage float64) tea.Cmd {
	return func() tea.Msg {
		if f == nil {
			return swapExecMsg{result: swap.FlowResult{Success: false, Err: "swap service not configured"}}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()
		res := f.Execute(ctx, privateKey, from, to, usd, slippage)
		return swapExecMsg{result: res}
	}
}

func thorQuoteCmd(f *swap.Flow, from swap.TokenInfo, usd, btc string) tea.Cmd {
	return func() tea.Msg {
		if f == nil {
			return swapbtcpanel.QuoteResultMsg{Err: fmt.Errorf("swap service not configured")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		q, err := f.GetThorchainQuote(ctx, from, usd, btc)
		return swapbtcpanel.QuoteResultMsg{Quote: q, Err: err}
	}
}

func thorEstimateCmd(f *swap.Flow, from swap.TokenInfo, usd string) tea.Cmd {
	return func() tea.Msg {
		if f == nil {
			return swapbtcpanel.EstimateResultMsg{Err: fmt.Errorf("swap service not configured")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		q, err := f.EstimateThorchain(ctx, from, usd)
		return swapbtcpanel.EstimateResultMsg{Quote: q, Err: err}
	}
}

func thorExecuteCmd(f *swap.Flow, privateKey string, from swap.TokenInfo, usd, btc string) tea.Cmd {
	return func() tea.Msg {
		if f == nil {
			return swapbtcpanel.ExecutionResultMsg{Result: swap.FlowResult{Success: false, Err: "swap service not configured"}}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
		defer cancel()
		res := f.ExecuteThorchain(ctx, privateKey, from, usd, btc)
		return swapbtcpanel.ExecutionResultMsg{Result: res}
	}
}

// explorerAPIKeyEnv is the env var read at fetch time. Empty value disables
// explorer-API token discovery (chain-config tokens only).
const explorerAPIKeyEnv = "LAZYSWAP_EXPLORER_API_KEY"

func fetchBalancesCmd(svc *balance.Service, address string) tea.Cmd {
	return func() tea.Msg {
		if svc == nil {
			return balancesFetchedMsg{walletAddress: address, err: fmt.Errorf("balance service not configured")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		bs, err := svc.FetchAll(ctx, address, os.Getenv(explorerAPIKeyEnv))
		return balancesFetchedMsg{walletAddress: address, balances: bs, err: err}
	}
}

// chainSwitchedMsg reports the result of re-dialing services for a new chain.
type chainSwitchedMsg struct {
	chainKey string
	balSvc   *balance.Service
	flowSvc  *swap.Flow
	err      error
}

// switchChainCmd re-dials the balance + swap RPC services for chainKey off the
// UI thread. The previous services are closed by the handler once these are
// ready, so a failed dial leaves the current chain untouched.
func switchChainCmd(chainKey string) tea.Cmd {
	return func() tea.Msg {
		balSvc, err := balance.New(chainKey)
		if err != nil {
			return chainSwitchedMsg{chainKey: chainKey, err: err}
		}
		flowSvc, err := swap.NewFlow(chainKey)
		if err != nil {
			balSvc.Close()
			return chainSwitchedMsg{chainKey: chainKey, err: err}
		}
		return chainSwitchedMsg{chainKey: chainKey, balSvc: balSvc, flowSvc: flowSvc}
	}
}

// ─── Update ──────────────────────────────────────────────────────────────────

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)
		return m, nil

	case walletsRefreshedMsg:
		if msg.err != nil {
			m.errMsg = msg.err.Error()
			return m, nil
		}
		m.wallets = msg.wallets
		m.panel.SetWallets(msg.wallets)
		if sel := m.panel.Selected(); sel != nil {
			m.current = sel
		} else if len(msg.wallets) > 0 {
			m.current = &msg.wallets[0]
		} else {
			m.current = nil
		}
		return m, m.balancesCmdForCurrent()

	case createdMsg:
		m.mode = modeNormal
		if msg.err != nil {
			m.errMsg = "create: " + msg.err.Error()
			return m, nil
		}
		return m, refreshWalletsCmd(m.svc)

	case importedMsg:
		if msg.err != nil {
			m.imp.SetErr("import: " + msg.err.Error())
			return m, nil
		}
		m.mode = modeNormal
		return m, refreshWalletsCmd(m.svc)

	case deletedMsg:
		m.mode = modeNormal
		if msg.err != nil {
			m.errMsg = "delete: " + msg.err.Error()
			return m, nil
		}
		return m, refreshWalletsCmd(m.svc)

	case balancesFetchedMsg:
		if msg.err != nil {
			m.tokens.SetError(msg.err.Error())
			return m, nil
		}
		m.balanceCache[msg.walletAddress] = msg.balances
		// Only paint the result if the cached fetch still matches the
		// selected wallet (the user may have moved on while we awaited RPC).
		if m.current != nil && m.current.Address == msg.walletAddress {
			m.tokens.SetBalances(msg.balances)
			m.swapbtc.SetBalances(msg.balances)
		}
		return m, nil

	case importoverlay.CancelMsg:
		m.mode = modeNormal
		return m, nil

	case importoverlay.SubmitMsg:
		if msg.Phrase == "" {
			m.imp.SetErr("empty mnemonic")
			return m, nil
		}
		return m, importCmd(m.svc, msg.Phrase)

	case swapoverlay.CancelMsg:
		m.mode = modeNormal
		return m, nil

	case swapoverlay.QuoteRequestMsg:
		if m.current == nil {
			return m, nil
		}
		return m, quoteSwapCmd(m.flowSvc, msg.From, msg.To, msg.USDAmount, m.current.Address, m.slippage)

	case swapoverlay.ExecuteRequestMsg:
		if m.current == nil {
			return m, nil
		}
		applog.Tracef("mainscreen — executing swap %s → %s $%s for %s",
			msg.From.Symbol, msg.To.Symbol, msg.USDAmount, m.current.Address)
		return m, executeSwapCmd(m.flowSvc, m.current.PrivateKey, msg.From, msg.To, msg.USDAmount, m.slippage)

	case swapQuoteMsg:
		m.swap, _ = m.swap.Update(swapoverlay.QuoteResultMsg{Quote: msg.quote, Err: msg.err})
		return m, nil

	case swapExecMsg:
		applog.Infof("swap result success=%v tx=%s err=%s", msg.result.Success, msg.result.TxHash, msg.result.Err)
		// Invalidate cached balances for the active wallet so the next view shows the new state.
		if m.current != nil {
			delete(m.balanceCache, m.current.Address)
		}
		m.swap, _ = m.swap.Update(swapoverlay.ExecutionResultMsg{Result: msg.result})
		return m, nil

	case swapbtcpanel.EstimateRequestMsg:
		if m.current == nil {
			return m, nil
		}
		return m, thorEstimateCmd(m.flowSvc, msg.From, msg.USDAmount)

	case swapbtcpanel.QuoteRequestMsg:
		if m.current == nil {
			return m, nil
		}
		return m, thorQuoteCmd(m.flowSvc, msg.From, msg.USDAmount, msg.BTCAddress)

	case swapbtcpanel.ExecuteRequestMsg:
		if m.current == nil {
			return m, nil
		}
		applog.Tracef("mainscreen — executing BTC swap %s → BTC $%s for %s",
			msg.From.Symbol, msg.USDAmount, m.current.Address)
		return m, thorExecuteCmd(m.flowSvc, m.current.PrivateKey, msg.From, msg.USDAmount, msg.BTCAddress)

	case swapbtcpanel.QuoteResultMsg, swapbtcpanel.ExecutionResultMsg, swapbtcpanel.TickMsg, swapbtcpanel.EstimateResultMsg:
		// Parent → panel results + countdown ticks reach the Swap BTC tab even
		// when it isn't focused, so an in-flight quote/swap finishes cleanly.
		var cmd tea.Cmd
		m.swapbtc, cmd = m.swapbtc.Update(msg)
		if r, ok := msg.(swapbtcpanel.ExecutionResultMsg); ok && r.Result.Success && m.current != nil {
			delete(m.balanceCache, m.current.Address)
		}
		return m, cmd

	case settingspanel.ShowWalletMsg:
		if m.current == nil {
			m.errMsg = "no wallet selected"
			return m, nil
		}
		m.mode = modeWalletQR
		m.walletCopyStatus = ""
		return m, nil

	case settingspanel.SlippageChangedMsg:
		m.slippage = msg.Value
		m.settings.SetSlippage(msg.Value)
		return m, nil

	case settingspanel.NetworkChangeMsg:
		if m.balSvc == nil {
			// No RPC services in this session (e.g. tests) — just record it.
			c := chain.Get(msg.ChainKey)
			m.chainKey = msg.ChainKey
			m.settings.SetNetwork(msg.ChainKey, c.Name)
			return m, nil
		}
		return m, switchChainCmd(msg.ChainKey)

	case chainSwitchedMsg:
		if msg.err != nil {
			m.errMsg = "network switch: " + msg.err.Error()
			return m, nil
		}
		if m.balSvc != nil {
			m.balSvc.Close()
		}
		if m.flowSvc != nil {
			m.flowSvc.Close()
		}
		m.chainKey = msg.chainKey
		m.balSvc = msg.balSvc
		m.flowSvc = msg.flowSvc
		c := chain.Get(msg.chainKey)
		m.settings.SetNetwork(msg.chainKey, c.Name)
		m.balanceCache = make(map[string][]balance.TokenBalance)
		m.errMsg = ""
		return m, m.balancesCmdForCurrent()

	case walletpanel.SelectionChangedMsg:
		m.current = msg.Wallet
		return m, m.balancesCmdForCurrent()
	}

	// Mode-specific dispatch.
	switch m.mode {
	case modeImport:
		var cmd tea.Cmd
		m.imp, cmd = m.imp.Update(msg)
		return m, cmd

	case modeSwap:
		var cmd tea.Cmd
		m.swap, cmd = m.swap.Update(msg)
		return m, cmd

	case modeConfirmCreate:
		if k, ok := msg.(tea.KeyMsg); ok {
			if k.String() == "y" || k.String() == "Y" {
				return m, createCmd(m.svc)
			}
			m.mode = modeNormal
		}
		return m, nil

	case modeConfirmDelete:
		if k, ok := msg.(tea.KeyMsg); ok {
			if k.String() == "y" || k.String() == "Y" {
				if m.current == nil {
					m.mode = modeNormal
					return m, nil
				}
				return m, deleteCmd(m.svc, m.current.ID)
			}
			m.mode = modeNormal
		}
		return m, nil

	case modeWalletQR:
		if k, ok := msg.(tea.KeyMsg); ok {
			switch k.String() {
			case "c", "y":
				if m.current != nil {
					if err := clipboard.WriteAll(m.current.Address); err != nil {
						m.walletCopyStatus = "copy failed: " + err.Error()
					} else {
						m.walletCopyStatus = "address copied to clipboard"
					}
				}
			case "esc", "q", "enter":
				m.mode = modeNormal
				m.walletCopyStatus = ""
			}
		}
		return m, nil
	}

	// Normal mode key bindings.
	if k, ok := msg.(tea.KeyMsg); ok {
		// When the active tab is editing a text field, keys go straight to it
		// before any global shortcut runs — otherwise letters like the 'c' in a
		// bech32 BTC address would be swallowed by create/delete/import/swap.
		if m.focus == focusRight && m.capturingInput() {
			return m.routeToActiveTab(msg)
		}
		// Number keys switch right-panel tabs, unless the active tab is
		// currently capturing text input (then the digit is a literal char).
		if !m.capturingInput() {
			if n, err := strconv.Atoi(k.String()); err == nil && m.tabExists(n) {
				m.activeTab = tab(n)
				m.focus = focusRight
				m.applyFocusStyles()
				return m, m.onTabActivated()
			}
		}
		switch k.String() {
		case "c":
			m.mode = modeConfirmCreate
			return m, nil
		case "d":
			if m.current != nil {
				m.mode = modeConfirmDelete
			}
			return m, nil
		case "i":
			m.imp = importoverlay.New()
			m.imp.SetSize(m.width, m.height)
			m.mode = modeImport
			return m, m.imp.Init()
		case "s":
			if m.current == nil || m.flowSvc == nil || m.balSvc == nil {
				return m, nil
			}
			cached, ok := m.balanceCache[m.current.Address]
			if !ok || len(cached) == 0 {
				m.errMsg = "swap: balances not loaded — press 'r' to refresh"
				return m, nil
			}
			m.errMsg = ""
			m.swap = swapoverlay.New(cached, m.balSvc.Chain())
			m.swap.SetSize(m.width, m.height)
			m.mode = modeSwap
			return m, m.swap.Init()
		case "r":
			return m, m.balancesCmdForCurrent()
		case "y":
			if m.current == nil {
				return m, nil
			}
			if err := clipboard.WriteAll(m.current.Address); err != nil {
				m.errMsg = "copy: " + err.Error()
				m.notice = ""
			} else {
				m.errMsg = ""
				m.notice = "copied " + shortAddr(m.current.Address)
			}
			return m, nil
		case "tab":
			if m.focus == focusLeft {
				m.focus = focusRight
			} else {
				m.focus = focusLeft
			}
			m.applyFocusStyles()
			return m, nil
		}
	}

	// Route to the focused panel.
	if m.focus == focusRight {
		return m.routeToActiveTab(msg)
	}
	var cmd tea.Cmd
	m.panel, cmd = m.panel.Update(msg)
	return m, cmd
}

// routeToActiveTab forwards a message to the currently selected right-panel tab.
func (m Model) routeToActiveTab(msg tea.Msg) (Model, tea.Cmd) {
	switch m.activeTab {
	case tabSettings:
		var cmd tea.Cmd
		m.settings, cmd = m.settings.Update(msg)
		return m, cmd
	case tabSwapBTC:
		var cmd tea.Cmd
		m.swapbtc, cmd = m.swapbtc.Update(msg)
		return m, cmd
	default:
		var cmd tea.Cmd
		m.tokens, cmd = m.tokens.Update(msg)
		return m, cmd
	}
}

// balancesCmdForCurrent returns a tea.Cmd that fetches balances for the
// active wallet (or uses the cache when fresh). Returns nil if no wallet is
// selected.
func (m *Model) balancesCmdForCurrent() tea.Cmd {
	if m.current == nil {
		return nil
	}
	addr := m.current.Address
	subtitle := fmt.Sprintf("%s · %s", shortAddr(addr), m.chainName())
	m.tokens.SetSubtitle(subtitle)

	if cached, ok := m.balanceCache[addr]; ok {
		m.tokens.SetBalances(cached)
		m.swapbtc.SetBalances(cached)
		return nil
	}
	m.tokens.SetLoading()
	return fetchBalancesCmd(m.balSvc, addr)
}

func (m Model) chainName() string {
	if m.balSvc == nil {
		return chain.Get(chain.DefaultKey).Name
	}
	return m.balSvc.Chain().Name
}

// ─── Tabs ──────────────────────────────────────────────────────────────────────

// availableTabs returns the enabled right-panel tabs in display order. Mirrors
// the FEATURES.tabs gate in src/common/features.ts. Settings (4) and Swap BTC
// (5) are appended as their panels land in later phases.
func (m Model) availableTabs() []rightpanel.Tab {
	return []rightpanel.Tab{
		{Num: int(tabTokens), Label: "Tokens"},
		{Num: int(tabSettings), Label: "Settings"},
		{Num: int(tabSwapBTC), Label: "Swap BTC"},
	}
}

// tabExists reports whether n maps to an enabled tab.
func (m Model) tabExists(n int) bool {
	for _, t := range m.availableTabs() {
		if t.Num == n {
			return true
		}
	}
	return false
}

// capturingInput reports whether the active tab is currently editing a text
// field — when true, number keys are typed into the field rather than treated
// as tab-switch shortcuts.
func (m Model) capturingInput() bool {
	switch m.activeTab {
	case tabSettings:
		return m.settings.Capturing()
	case tabSwapBTC:
		return m.swapbtc.Capturing()
	default:
		return false
	}
}

// rightColumn renders the tab header stacked above the active tab's body.
func (m Model) rightColumn() string {
	header := theme.Text().Render(rightpanel.Header(int(m.activeTab), m.availableTabs()))
	return lipgloss.JoinVertical(lipgloss.Left, header, m.activeTabView())
}

// activeTabView returns the View of whichever tab is currently selected.
func (m Model) activeTabView() string {
	switch m.activeTab {
	case tabSettings:
		return m.settings.View()
	case tabSwapBTC:
		return m.swapbtc.View()
	default:
		return m.tokens.View()
	}
}

// onTabActivated returns any command to run when a tab becomes active (e.g.
// loading balances for the tokens tab). Later phases hook settings / swapBTC.
func (m *Model) onTabActivated() tea.Cmd {
	switch m.activeTab {
	case tabTokens:
		return m.balancesCmdForCurrent()
	case tabSwapBTC:
		// Seed the source list from cache; fetch if we have nothing yet.
		if m.current != nil {
			if cached, ok := m.balanceCache[m.current.Address]; ok {
				m.swapbtc.SetBalances(cached)
			}
		}
		return m.balancesCmdForCurrent()
	default:
		return nil
	}
}

// ─── View ────────────────────────────────────────────────────────────────────

func (m Model) View() string {
	if m.mode == modeImport {
		return m.imp.View()
	}
	if m.mode == modeSwap {
		return m.swap.View()
	}
	if m.mode == modeWalletQR {
		return m.walletQRView()
	}

	body := lipgloss.JoinHorizontal(lipgloss.Top, m.panel.View(), m.rightColumn())

	if m.mode == modeConfirmCreate || m.mode == modeConfirmDelete {
		var prompt string
		switch m.mode {
		case modeConfirmCreate:
			prompt = "Create a new wallet? (y/n)"
		case modeConfirmDelete:
			if m.current == nil {
				prompt = ""
			} else {
				prompt = fmt.Sprintf("Delete wallet %s? (y/n)", shortAddr(m.current.Address))
			}
		}
		overlay := theme.Border(true).
			Padding(1, 3).
			Background(theme.YellowSel).
			Render(theme.Text().Bold(true).Render(prompt))
		if m.width > 0 && m.height > 0 {
			return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, overlay)
		}
		return overlay
	}

	// Always render an error row (empty when no error) so panel heights stay
	// stable as errors come and go. The same row doubles as a transient notice
	// line (e.g. "copied …") when there's no error to show.
	errLine := ""
	if m.errMsg != "" {
		errLine = theme.Error().Render(m.errMsg)
	} else if m.notice != "" {
		errLine = theme.Text().Render(m.notice)
	}
	status := m.renderStatusBar()

	return lipgloss.JoinVertical(lipgloss.Left, body, errLine, status)
}

// walletQRView renders the full-screen, centered overlay showing the active
// wallet's address as scannable QR plus the raw string, with a copy hint.
// The QR is drawn inverse (dark modules on a yellow field) so it keeps the
// conventional dark-on-light orientation that every scanner expects.
func (m Model) walletQRView() string {
	if m.current == nil {
		return ""
	}
	addr := m.current.Address

	var qr string
	if q, err := qrcode.New(addr, qrcode.Medium); err != nil {
		qr = theme.Error().Render("QR error: " + err.Error())
	} else {
		qr = theme.Text().Render(q.ToSmallString(true))
	}

	status := theme.Dim().Render("c: copy   esc: close")
	if m.walletCopyStatus != "" {
		status = theme.Text().Bold(true).Render(m.walletCopyStatus)
	}

	content := lipgloss.JoinVertical(lipgloss.Center,
		theme.Text().Bold(true).Render("Wallet Address"),
		"",
		qr,
		theme.Text().Render(addr),
		"",
		status,
	)
	box := theme.Border(true).Padding(1, 3).Render(content)
	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
	}
	return box
}

// renderStatusBar returns the bottom hint line, mirroring TS status-bar.ts.
func (m Model) renderStatusBar() string {
	api := "off"
	if os.Getenv(explorerAPIKeyEnv) != "" {
		api = "on"
	}

	var hint string
	switch m.mode {
	case modeConfirmCreate, modeConfirmDelete:
		hint = "y: confirm  |  n/Esc: cancel"
	case modeImport:
		hint = "Enter: import  |  Esc: cancel"
	case modeSwap:
		hint = "Esc: back"
	case modeWalletQR:
		hint = "c: copy  |  Esc: close"
	default:
		switch m.focus {
		case focusLeft:
			hint = fmt.Sprintf("Tab: focus  |  c/d/i  y:copy  |  j/k  |  ^C  |  API:%s", api)
		case focusRight:
			hint = fmt.Sprintf("Tab: focus  |  s:swap  r:refresh  y:copy  |  j/k  |  ^C  |  API:%s", api)
		}
	}

	style := theme.Dim()
	if m.width > 0 {
		style = style.Width(m.width)
	}
	return style.Render(" " + hint)
}

func shortAddr(a string) string {
	if len(a) <= 12 {
		return a
	}
	return fmt.Sprintf("%s…%s", a[:6], a[len(a)-4:])
}
