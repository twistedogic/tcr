package main

import (
	"context"
	"flag"
	"os"
	"path/filepath"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/charmbracelet/wish/bubbletea"
	"github.com/google/subcommands"
)

type cmdFinishedMsg struct{ err error }

func wrapError(err error) tea.Cmd {
	return func() tea.Msg {
		return cmdFinishedMsg{err: err}
	}
}

func interactive(sess ssh.Session, dir string, cmd string, args ...string) tea.Cmd {
	callback := func(err error) tea.Msg {
		return cmdFinishedMsg{err: err}
	}
	if sess != nil {
		wishCmd := wish.Command(sess, cmd, args...)
		if dir != "" {
			wishCmd.SetDir("")
		}
		return tea.Exec(wishCmd, callback)
	}
	c := command(context.Background(), dir, cmd, args...)
	return tea.ExecProcess(c, callback)
}

func cloneForm() *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Key("owner").Title("owner").Validate(huh.ValidateNotEmpty()),
			huh.NewInput().Key("repo").Title("repo").Validate(huh.ValidateNotEmpty()),
		),
	)
}

type keyMap struct {
	New     key.Binding
	Explore key.Binding
	Quit    key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.New, k.Explore, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{k.ShortHelp()}
}

func defaultKeyMap() keyMap {
	return keyMap{
		New:     key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "clone new repo")),
		Explore: key.NewBinding(key.WithKeys("e", "enter"), key.WithHelp("e/enter", "explore repo")),
		Quit:    key.NewBinding(key.WithKeys("q", "esc"), key.WithHelp("q/esc", "exit")),
	}
}

type state uint

const (
	mainState state = iota
	newRepoState
)

type model struct {
	workspace string
	keyMap    keyMap
	err       error
	sess      ssh.Session
	errStyle  lipgloss.Style
	form      *huh.Form
	state     state
}

func NewModel(workspace string, sess ssh.Session, renderer *lipgloss.Renderer) tea.Model {
	return &model{
		workspace: workspace,
		sess:      sess,
		keyMap:    defaultKeyMap(),
		errStyle:  renderer.NewStyle().Foreground(lipgloss.Color("3")),
		form:      cloneForm(),
	}
}

func NewTeaHandler(workspace string) bubbletea.Handler {
	return func(s ssh.Session) (tea.Model, []tea.ProgramOption) {
		renderer := bubbletea.MakeRenderer(s)
		m := NewModel(workspace, s, renderer)
		return m, []tea.ProgramOption{tea.WithAltScreen()}
	}
}

func (m *model) Init() tea.Cmd { return nil }

func (m *model) formUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.form.State {
	case huh.StateAborted:
		m.form = cloneForm()
		m.state = mainState
	case huh.StateCompleted:
		repo := m.form.Get("repo").(string)
		owner := m.form.Get("owner").(string)
		m.state = mainState
		m.form = cloneForm()
		if err := clone(context.Background(), filepath.Join(m.workspace, "repo"), owner, repo); err != nil {
			return m, wrapError(err)
		}
	default:
		form, cmd := m.form.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			m.form = f
			return m, cmd
		}
	}
	return m, nil
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.state {
	case newRepoState:
		return m.formUpdate(msg)
	default:
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch {
			case key.Matches(msg, m.keyMap.New):
				m.form = cloneForm()
				m.state = newRepoState
				return m, nil
			case key.Matches(msg, m.keyMap.Explore):
				return m, interactive(m.sess, filepath.Join(m.workspace, "worktree"), "opencode")
			case key.Matches(msg, m.keyMap.Quit):
				return m, tea.Quit
			}
		case cmdFinishedMsg:
			m.err = msg.err
			return m, nil
		}
	}
	return m, nil
}

func (m *model) View() string {
	switch m.state {
	case newRepoState:
		return m.form.View()
	}
	if m.err != nil {
		return m.errStyle.Render(m.err.Error() + "\n")
	}
	return help.New().View(m.keyMap)
}

func bootstrapWorkspace(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, "repo"), 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, "worktree"), 0755); err != nil {
		return err
	}
	return nil
}

type appCmd struct{ workspace string }

func (*appCmd) Name() string     { return "start" }
func (*appCmd) Synopsis() string { return "start local process" }
func (*appCmd) Usage() string    { return "" }
func (a *appCmd) SetFlags(f *flag.FlagSet) {
	home, _ := os.UserHomeDir()
	ws := filepath.Join(home, ".local", "tcr")
	f.StringVar(&a.workspace, "workspace", ws, "dir for git worktree")
}
func (a *appCmd) Execute(_ context.Context, f *flag.FlagSet, _ ...any) subcommands.ExitStatus {
	if err := bootstrapWorkspace(a.workspace); err != nil {
		return subcommands.ExitFailure
	}
	m := NewModel(a.workspace, nil, lipgloss.DefaultRenderer())
	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		return subcommands.ExitFailure
	}
	return subcommands.ExitSuccess
}
