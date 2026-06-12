// LazySwap TUI — Go rewrite entry point.
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/applog"
	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/tui"
	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/wallet"
)

func main() {
	dao, err := wallet.Open()
	if err != nil {
		applog.Error("open dao", err)
		fmt.Fprintf(os.Stderr, "open dao: %v\n", err)
		os.Exit(1)
	}
	defer dao.Close()

	root, err := tui.NewRoot(dao, "")
	if err != nil {
		applog.Error("build root model", err)
		fmt.Fprintf(os.Stderr, "build root model: %v\n", err)
		os.Exit(1)
	}

	applog.Info("lazyswap starting")
	if _, err := tea.NewProgram(root, tea.WithAltScreen()).Run(); err != nil {
		applog.Error("tui exited with error", err)
		fmt.Fprintf(os.Stderr, "tui: %v\n", err)
		os.Exit(1)
	}
	applog.Info("lazyswap exited")
}
