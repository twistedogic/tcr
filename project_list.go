package main

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

type ProjectAction int

const (
	ProjectActionNone ProjectAction = iota
	ProjectActionExplore
	ProjectActionClone
	ProjectActionDelete
	ProjectActionQuit
)

type projectSelectedMsg struct {
	action  ProjectAction
	project *Project
}

type projectKeyMap struct {
	Explore key.Binding
	Clone   key.Binding
	Delete  key.Binding
	Quit    key.Binding
}

func (k projectKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Explore, k.Clone, k.Delete, k.Quit}
}

func (k projectKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{k.ShortHelp()}
}

func defaultProjectKeyMap() projectKeyMap {
	return projectKeyMap{
		Explore: key.NewBinding(key.WithKeys("e", "enter"), key.WithHelp("e/enter", "explore")),
		Clone:   key.NewBinding(key.WithKeys("c", "n"), key.WithHelp("c/n", "clone")),
		Delete:  key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete")),
		Quit:    key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
	}
}

type ProjectList struct {
	list   list.Model
	keyMap projectKeyMap
}

func NewProjectList(projects []*Project, width, height int) *ProjectList {
	items := make([]list.Item, len(projects))
	for i, p := range projects {
		items[i] = p
	}

	keyMap := defaultProjectKeyMap()
	l := list.New(items, list.NewDefaultDelegate(), width, height)
	l.Title = "Projects"
	l.SetShowHelp(true)
	l.SetShowStatusBar(true)
	l.SetStatusBarItemName("project", "projects")
	l.AdditionalFullHelpKeys = keyMap.ShortHelp
	l.AdditionalShortHelpKeys = keyMap.ShortHelp
	if len(projects) == 0 {
		l.SetShowFilter(false)
	}

	return &ProjectList{
		list:   l,
		keyMap: keyMap,
	}
}

func (p *ProjectList) Init() tea.Cmd {
	return nil
}

func (p *ProjectList) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, p.keyMap.Explore):
			if selected, ok := p.list.SelectedItem().(*Project); ok {
				return p, func() tea.Msg {
					return projectSelectedMsg{action: ProjectActionExplore, project: selected}
				}
			}
		case key.Matches(msg, p.keyMap.Clone):
			return p, func() tea.Msg {
				return projectSelectedMsg{action: ProjectActionClone}
			}
		case key.Matches(msg, p.keyMap.Delete):
			if selected, ok := p.list.SelectedItem().(*Project); ok {
				return p, func() tea.Msg {
					return projectSelectedMsg{action: ProjectActionDelete, project: selected}
				}
			}
		case key.Matches(msg, p.keyMap.Quit):
			return p, func() tea.Msg {
				return projectSelectedMsg{action: ProjectActionQuit}
			}
		}

	case tea.WindowSizeMsg:
		p.list.SetSize(msg.Width, msg.Height)
	}

	var cmd tea.Cmd
	p.list, cmd = p.list.Update(msg)
	return p, cmd
}

func (p *ProjectList) View() string {
	return p.list.View()
}

func (p *ProjectList) SetItems(projects []*Project) {
	items := make([]list.Item, len(projects))
	for i, proj := range projects {
		items[i] = proj
	}
	p.list.SetItems(items)
}
