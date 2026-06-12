// Package tui owns the root Bubble Tea model.
//
// The root holds a screen-state machine (login → main). On a successful
// login the crypto service is wired into a wallet.Service which is handed
// to the main screen.
package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/applog"
	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/balance"
	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/chain"
	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/crypto"
	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/swap"
	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/tui/screens/login"
	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/tui/screens/mainscreen"
	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/wallet"
)

type screen int

const (
	screenLogin screen = iota
	screenMain
)

// Root is the top-level Bubble Tea model.
type Root struct {
	dao      *wallet.DAO
	chainKey string
	screen   screen
	login    login.Model
	main     *mainscreen.Model
	svc      *crypto.Service
	balSvc   *balance.Service
	flowSvc  *swap.Flow

	width, height int
}

// NewRoot constructs the root model with the login screen active. chainKey
// selects the EVM chain for balance + (eventual) swap RPC calls; pass an
// empty string to use chain.DefaultKey.
func NewRoot(dao *wallet.DAO, chainKey string) (Root, error) {
	if chainKey == "" {
		chainKey = chain.DefaultKey
	}
	lm, err := login.New(dao)
	if err != nil {
		return Root{}, err
	}
	return Root{
		dao:      dao,
		chainKey: chainKey,
		screen:   screenLogin,
		login:    lm,
	}, nil
}

func (r Root) Init() tea.Cmd { return r.login.Init() }

func (r Root) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		r.width, r.height = msg.Width, msg.Height
		switch r.screen {
		case screenLogin:
			r.login, _ = r.login.Update(msg)
		case screenMain:
			if r.main != nil {
				m, _ := r.main.Update(msg)
				r.main = &m
			}
		}
		return r, nil

	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			return r, tea.Quit
		}

	case login.LoginSuccessMsg:
		r.svc = msg.Service
		walletSvc := wallet.NewService(r.dao, msg.Service)

		// Dial the chain RPC for balance lookups. Soft-fail: a bad RPC
		// shouldn't gate login — the tokens panel renders an error state.
		balSvc, err := balance.New(r.chainKey)
		if err != nil {
			applog.Error("balance.New", err)
			balSvc = nil
		}
		r.balSvc = balSvc

		flowSvc, err := swap.NewFlow(r.chainKey)
		if err != nil {
			applog.Error("swap.NewFlow", err)
			flowSvc = nil
		}
		r.flowSvc = flowSvc

		main := mainscreen.New(walletSvc, balSvc, flowSvc, r.chainKey)
		main.SetSize(r.width, r.height)
		r.main = &main
		r.screen = screenMain
		return r, r.main.Init()
	}

	switch r.screen {
	case screenLogin:
		var cmd tea.Cmd
		r.login, cmd = r.login.Update(msg)
		return r, cmd
	case screenMain:
		if r.main == nil {
			return r, nil
		}
		m, cmd := r.main.Update(msg)
		r.main = &m
		return r, cmd
	}
	return r, nil
}

func (r Root) View() string {
	switch r.screen {
	case screenLogin:
		return r.login.View()
	case screenMain:
		if r.main == nil {
			return ""
		}
		return r.main.View()
	}
	return ""
}
