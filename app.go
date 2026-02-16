package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/charmbracelet/wish/bubbletea"
	"github.com/google/subcommands"
)

type cmdFinishedMsg struct{ err error }

func interactive(sess ssh.Session, dir string, cmd string, args ...string) tea.Cmd {
	callback := func(err error) tea.Msg {
		return cmdFinishedMsg{err: err}
	}
	if sess != nil {
		wishCmd := wish.Command(sess, cmd, args...)
		if dir != "" {
			wishCmd.SetDir(dir)
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
		).Title("Clone git repository"),
	)
}

func newWorktreeForm(repoName string) *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Key("name").Title("worktree name").Validate(huh.ValidateNotEmpty()),
		).Title(fmt.Sprintf("%s – add new worktree", repoName)),
	)
}

func deleteConfirmForm(kind, name string) *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Key("confirm").
				Title("Delete " + kind + " " + name + "?").
				Affirmative("Yes").
				Negative("No"),
		),
	)
}

type state uint

const (
	mainState state = iota
	newRepoState
	projectState
	deleteWorktreeState
	deleteProjectState
	newWorktreeState
)

type model struct {
	workspace string
	err       error
	sess      ssh.Session
	errStyle  lipgloss.Style
	form      *huh.Form
	state     state
	spinner   spinner.Model
	loading   bool

	// project list (mainState)
	projectList     *ProjectList
	selectedProject *Project

	// worktree list (projectState)
	project          *Project
	wtList           *WorktreeList
	selectedWorktree *Worktree

	client *GitHubPRClient
}

func NewModel(workspace string, sess ssh.Session, renderer *lipgloss.Renderer) tea.Model {
	s := spinner.New()
	return &model{
		workspace: workspace,
		sess:      sess,
		errStyle:  renderer.NewStyle().Foreground(lipgloss.Color("3")),
		spinner:   s,
		loading:   true,
	}
}

func NewTeaHandler(workspace string) bubbletea.Handler {
	return func(s ssh.Session) (tea.Model, []tea.ProgramOption) {
		renderer := bubbletea.MakeRenderer(s)
		m := NewModel(workspace, s, renderer)
		return m, []tea.ProgramOption{tea.WithAltScreen()}
	}
}

type projectsLoadedMsg struct {
	projects []*Project
	err      error
}

func (m *model) loadProjects() tea.Msg {
	repoDir := filepath.Join(m.workspace, "repo")
	wtDir := filepath.Join(m.workspace, "worktree")
	projects, err := LoadProjects(context.Background(), repoDir, wtDir)
	return projectsLoadedMsg{projects: projects, err: err}
}

func (m *model) startLoadProjects() tea.Cmd {
	m.loading = true
	return tea.Batch(m.spinner.Tick, m.loadProjects)
}

func (m *model) Init() tea.Cmd { return tea.Batch(m.spinner.Tick, m.loadProjects) }

func (m *model) setForm(form *huh.Form, s state) tea.Cmd {
	m.form = form
	m.state = s
	if form != nil {
		return form.Init()
	}
	return nil
}

// handleFormDone processes a form that has reached StateCompleted or
// StateAborted and transitions the model back to the appropriate parent state.
func (m *model) handleFormDone() (tea.Model, tea.Cmd) {
	switch m.form.State {
	case huh.StateAborted:
		switch m.state {
		case newRepoState:
			m.setForm(nil, mainState)
			return m, m.startLoadProjects()
		case newWorktreeState:
			m.setForm(nil, projectState)
		case deleteWorktreeState:
			m.setForm(nil, projectState)
		case deleteProjectState:
			m.selectedProject = nil
			m.setForm(nil, mainState)
			return m, m.startLoadProjects()
		}
	case huh.StateCompleted:
		switch m.state {
		case newRepoState:
			repo := m.form.Get("repo").(string)
			owner := m.form.Get("owner").(string)
			m.setForm(nil, mainState)
			if err := clone(context.Background(), filepath.Join(m.workspace, "repo"), owner, repo); err != nil {
				m.err = err
			}
			return m, m.startLoadProjects()
		case newWorktreeState:
			name := m.form.Get("name").(string)
			m.setForm(nil, projectState)
			if err := m.project.AddWorktree(context.Background(), name); err != nil {
				m.err = err
				return m, nil
			}
			m.wtList.SetItems(m.project.worktrees)
			return m, m.startLoadProjects()
		case deleteWorktreeState:
			confirmed := m.form.Get("confirm").(bool)
			m.setForm(nil, projectState)
			if confirmed {
				if err := m.project.DeleteWorktree(context.Background(), m.selectedWorktree.Name); err != nil {
					m.err = err
					m.selectedWorktree = nil
					return m, nil
				}
				m.wtList.SetItems(m.project.worktrees)
			}
			m.selectedWorktree = nil
		case deleteProjectState:
			confirmed := m.form.Get("confirm").(bool)
			repoPath := m.selectedProject.repoPath
			wtPath := m.selectedProject.worktreePath
			m.selectedProject = nil
			m.setForm(nil, mainState)
			if confirmed {
				if err := os.RemoveAll(repoPath); err != nil {
					m.err = err
				}
				if err := os.RemoveAll(wtPath); err != nil {
					m.err = err
				}
			}
			return m, m.startLoadProjects()
		}
	}
	return m, nil
}

func (m *model) formUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	// If the form already reached a terminal state (e.g. from a previous
	// update cycle), handle it immediately.
	if m.form.State == huh.StateAborted || m.form.State == huh.StateCompleted {
		return m.handleFormDone()
	}

	// Forward the message to the form.
	form, cmd := m.form.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.form = f
	}

	// Re-check: the update may have transitioned the form to a terminal
	// state. Handle it now so that View() never renders an empty completed
	// form (which causes the blank-screen flash).
	if m.form.State == huh.StateAborted || m.form.State == huh.StateCompleted {
		mdl, formDone := m.handleFormDone()
		return mdl, tea.Batch(cmd, formDone)
	}

	return m, cmd
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Forward spinner tick messages while loading.
	if _, ok := msg.(spinner.TickMsg); ok && m.loading {
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	// Safety net: if a cmdFinishedMsg with an error arrives while still in a
	// form state, force-recover to the appropriate parent state. Under normal
	// operation the per-branch fixes handle this; this catch-all protects
	// against future regressions.
	if msg, ok := msg.(cmdFinishedMsg); ok && msg.err != nil {
		switch m.state {
		case newRepoState, deleteProjectState:
			m.err = msg.err
			m.form = nil
			m.selectedProject = nil
			m.state = mainState
			return m, m.startLoadProjects()
		case newWorktreeState, deleteWorktreeState:
			m.err = msg.err
			m.form = nil
			m.selectedWorktree = nil
			m.state = projectState
			return m, nil
		}
	}

	switch m.state {
	case newRepoState, newWorktreeState, deleteWorktreeState, deleteProjectState:
		return m.formUpdate(msg)
	case projectState:
		switch msg := msg.(type) {
		case worktreeSelectedMsg:
			switch msg.action {
			case ActionReview:
				m.selectedWorktree = msg.worktree
				return m, interactive(m.sess, m.selectedWorktree.Path, "tuicr", "--stdout")
			case ActionInteract:
				m.selectedWorktree = msg.worktree
				return m, interactive(m.sess, m.selectedWorktree.Path, "opencode")
			case ActionCreate:
				return m, m.setForm(newWorktreeForm(m.project.Title()), newWorktreeState)
			case ActionDelete:
				m.selectedWorktree = msg.worktree
				return m, m.setForm(deleteConfirmForm("worktree", msg.worktree.Name), deleteWorktreeState)
			case ActionBack:
				m.project = nil
				m.wtList = nil
				m.state = mainState
				return m, m.startLoadProjects()
			}
		case cmdFinishedMsg:
			if msg.err != nil {
				m.err = msg.err
			}
			// Refresh worktree list after command completes
			if m.project != nil {
				if err := m.project.Refresh(context.Background()); err != nil {
					m.err = err
				} else {
					m.wtList.SetItems(m.project.worktrees)
				}
			}
			return m, nil
		}

		var cmd tea.Cmd
		mdl, cmd := m.wtList.Update(msg)
		if wtl, ok := mdl.(*WorktreeList); ok {
			m.wtList = wtl
		}
		return m, cmd

	default: // mainState
		switch msg := msg.(type) {
		case projectsLoadedMsg:
			m.loading = false
			if msg.err != nil {
				m.err = msg.err
			} else {
				m.err = nil
			}
			m.projectList = NewProjectList(msg.projects, 80, 20)
			if len(msg.projects) == 0 && msg.err == nil {
				return m, m.setForm(cloneForm(), newRepoState)
			}
			return m, nil
		case projectSelectedMsg:
			switch msg.action {
			case ProjectActionExplore:
				m.project = msg.project
				m.wtList = NewWorktreeList(m.project.worktrees, 80, 20)
				m.state = projectState
				if len(m.project.worktrees) == 0 {
					return m, m.setForm(newWorktreeForm(m.project.Title()), newWorktreeState)
				}
				return m, nil
			case ProjectActionClone:
				return m, m.setForm(cloneForm(), newRepoState)
			case ProjectActionDelete:
				m.selectedProject = msg.project
				return m, m.setForm(deleteConfirmForm("project", msg.project.Title()), deleteProjectState)
			case ProjectActionQuit:
				return m, tea.Quit
			}
		case cmdFinishedMsg:
			m.err = msg.err
			return m, nil
		}

		// Delegate to project list if available
		if m.projectList != nil {
			var cmd tea.Cmd
			mdl, cmd := m.projectList.Update(msg)
			if pl, ok := mdl.(*ProjectList); ok {
				m.projectList = pl
			}
			return m, cmd
		}
		if m.wtList != nil {
			mdl, cmd := m.wtList.Update(msg)
			if wl, ok := mdl.(*WorktreeList); ok {
				m.wtList = wl
			}
			return m, cmd
		}
	}
	return m, nil
}

func (m *model) View() string {
	switch m.state {
	case newRepoState, newWorktreeState, deleteWorktreeState, deleteProjectState:
		return m.form.View()
	case projectState:
		return m.wtList.View()
	}
	if m.err != nil && m.projectList != nil {
		return m.errStyle.Render(m.err.Error()+"\n\n") + m.projectList.View()
	}
	if m.err != nil {
		return m.errStyle.Render(m.err.Error() + "\n")
	}
	if m.projectList != nil {
		return m.projectList.View()
	}
	if m.loading {
		return m.spinner.View() + " Loading projects..."
	}
	return ""
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
	ws := filepath.Join(home, ".local", "share", "tcr")
	f.StringVar(&a.workspace, "workspace", ws, "dir for git worktree")
}
func (a *appCmd) Execute(_ context.Context, f *flag.FlagSet, _ ...any) subcommands.ExitStatus {
	if err := bootstrapWorkspace(a.workspace); err != nil {
		return subcommands.ExitFailure
	}
	m := NewModel(a.workspace, nil, lipgloss.DefaultRenderer())

	// Run the TUI
	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		return subcommands.ExitFailure
	}

	return subcommands.ExitSuccess
}
