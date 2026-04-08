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
			huh.NewInput().Key("branch").Title("branch").Placeholder("main").Validate(huh.ValidateNotEmpty()),
		).Title("Clone git repository"),
	)
}

func checkoutForm(repoName string) *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Key("name").Title("branch").Placeholder("e.g. main, feature/my-thing").Validate(huh.ValidateNotEmpty()),
		).Title(fmt.Sprintf("%s – create or checkout branch", repoName)),
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
	checkoutState
	deleteProjectState
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

	projectList     *ProjectList
	selectedProject *Project
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
	projects, err := LoadProjects(context.Background(), m.workspace)
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

func (m *model) handleFormDone() (tea.Model, tea.Cmd) {
	switch m.form.State {
	case huh.StateAborted:
		m.selectedProject = nil
		m.setForm(nil, mainState)
		return m, m.startLoadProjects()
	case huh.StateCompleted:
		switch m.state {
		case newRepoState:
			repo := m.form.Get("repo").(string)
			owner := m.form.Get("owner").(string)
			branch := m.form.Get("branch").(string)
			m.setForm(nil, mainState)
			if err := clone(context.Background(), m.workspace, owner, repo, branch); err != nil {
				m.err = err
			}
			return m, m.startLoadProjects()
		case checkoutState:
			name := m.form.Get("name").(string)
			p := m.selectedProject
			m.selectedProject = nil
			m.setForm(nil, mainState)
			if p != nil {
				if err := p.AddWorktree(context.Background(), name); err != nil {
					m.err = err
				}
			}
			return m, m.startLoadProjects()
		case deleteProjectState:
			confirmed := m.form.Get("confirm").(bool)
			projectPath := m.selectedProject.path
			m.selectedProject = nil
			m.setForm(nil, mainState)
			if confirmed {
				if err := os.RemoveAll(projectPath); err != nil {
					m.err = err
				}
			}
			return m, m.startLoadProjects()
		}
	}
	return m, nil
}

func (m *model) formUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.form.State == huh.StateAborted || m.form.State == huh.StateCompleted {
		return m.handleFormDone()
	}
	form, cmd := m.form.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.form = f
	}
	if m.form.State == huh.StateAborted || m.form.State == huh.StateCompleted {
		mdl, formDone := m.handleFormDone()
		return mdl, tea.Batch(cmd, formDone)
	}
	return m, cmd
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(spinner.TickMsg); ok && m.loading {
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	if msg, ok := msg.(cmdFinishedMsg); ok && msg.err != nil {
		m.err = msg.err
		m.form = nil
		m.selectedProject = nil
		m.state = mainState
		return m, m.startLoadProjects()
	}

	switch m.state {
	case newRepoState, checkoutState, deleteProjectState:
		return m.formUpdate(msg)
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
			case ProjectActionReview:
				return m, interactive(m.sess, msg.project.path, "tuicr", "--stdout")
			case ProjectActionInteract:
				return m, interactive(m.sess, msg.project.path, cfg.Interactive.Agent, cfg.Interactive.Args...)
			case ProjectActionCheckout:
				m.selectedProject = msg.project
				return m, m.setForm(checkoutForm(msg.project.Title()), checkoutState)
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

		if m.projectList != nil {
			var cmd tea.Cmd
			mdl, cmd := m.projectList.Update(msg)
			if pl, ok := mdl.(*ProjectList); ok {
				m.projectList = pl
			}
			return m, cmd
		}
	}
	return m, nil
}

func (m *model) View() string {
	switch m.state {
	case newRepoState, checkoutState, deleteProjectState:
		return m.form.View()
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
	return os.MkdirAll(dir, 0755)
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
	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		return subcommands.ExitFailure
	}
	return subcommands.ExitSuccess
}
