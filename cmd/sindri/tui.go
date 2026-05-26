package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/flo-at/sindri/internal/adapter"
	_ "github.com/flo-at/sindri/internal/adapter/claudecode"
	"github.com/flo-at/sindri/internal/app"
	"github.com/flo-at/sindri/internal/config"
	"github.com/flo-at/sindri/internal/event"
	"github.com/flo-at/sindri/internal/features"
	"github.com/flo-at/sindri/internal/keymap"
	"github.com/flo-at/sindri/internal/plugin"
	"github.com/flo-at/sindri/internal/plugins/conversations"
	"github.com/flo-at/sindri/internal/plugins/filebrowser"
	"github.com/flo-at/sindri/internal/plugins/gitstatus"
	"github.com/flo-at/sindri/internal/plugins/tdmonitor"
	"github.com/flo-at/sindri/internal/plugins/workspace"
	"github.com/flo-at/sindri/internal/state"
	"github.com/flo-at/sindri/internal/styles"
	"github.com/flo-at/sindri/internal/theme"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func newTuiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Launch the Sindri TUI dashboard",
		RunE:  runTui,
	}
}

func runTui(cmd *cobra.Command, args []string) error {
	cfg, _ := config.Load()
	features.Init(cfg)
	_ = state.Init()

	// Logger (discard — TUI handles its own output)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Event dispatcher
	dispatcher := event.NewWithLogger(logger)
	defer dispatcher.Close()

	// Resolve project root
	workDir, err := filepath.Abs(".")
	if err != nil {
		return fmt.Errorf("failed to resolve working directory: %w", err)
	}
	if gitRoot, err := exec.Command("git", "-C", workDir, "rev-parse", "--show-toplevel").Output(); err == nil {
		workDir = strings.TrimSpace(string(gitRoot))
	}
	projectRootPath := app.GetMainWorktreePath(workDir)
	if projectRootPath == "" {
		projectRootPath = workDir
	}

	// Theme
	resolved := theme.ResolveTheme(cfg, workDir)
	theme.ApplyResolved(resolved)
	styles.PillTabsEnabled = cfg.UI.NerdFontsEnabled

	// Keymap
	km := keymap.NewRegistry()
	keymap.RegisterDefaults(km)

	// Plugin context
	pluginCtx := &plugin.Context{
		WorkDir:     workDir,
		ProjectRoot: projectRootPath,
		ConfigDir:   config.ConfigPath(),
		Config:      cfg,
		Adapters:    adapter.AllAdapters(),
		EventBus:    dispatcher,
		Logger:      logger,
		Keymap:      km,
	}

	// Plugin registry
	registry := plugin.NewRegistry(pluginCtx)

	if cfg.Plugins.TDMonitor.Enabled {
		_ = registry.Register(tdmonitor.New())
	}
	if cfg.Plugins.GitStatus.Enabled {
		_ = registry.Register(gitstatus.New())
	}
	if cfg.Plugins.FileBrowser.Enabled {
		_ = registry.Register(filebrowser.New())
	}
	if cfg.Plugins.Conversations.Enabled {
		_ = registry.Register(conversations.New())
	}
	_ = registry.Register(workspace.New())

	// Keymap overrides
	for key, cmdID := range cfg.Keymap.Overrides {
		km.SetUserOverride(key, cmdID)
	}

	// Create and run
	initialPluginID := state.GetActivePlugin(projectRootPath)
	model := app.New(registry, km, cfg, "", workDir, projectRootPath, initialPluginID)

	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return fmt.Errorf("sindri tui requires an interactive terminal")
	}

	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseAllMotion())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}
	return nil
}
