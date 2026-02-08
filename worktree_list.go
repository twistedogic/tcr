package main

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

type WorktreeAction int

const (
	ActionNone WorktreeAction = iota
	ActionReview
	ActionInteract
	ActionCreate
	ActionDelete
	ActionBack
)

type worktreeKeyMap struct {
	Review   key.Binding
	Interact key.Binding
	Create   key.Binding
	Delete   key.Binding
	Back     key.Binding
}

func (k worktreeKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Review, k.Interact, k.Create, k.Delete, k.Back}
}

func (k worktreeKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{k.ShortHelp()}
}

func defaultWorktreeKeyMap() worktreeKeyMap {
	return worktreeKeyMap{
		Review:   key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "review")),
		Interact: key.NewBinding(key.WithKeys("i", "enter"), key.WithHelp("i/enter", "interact")),
		Create:   key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "create")),
		Delete:   key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete")),
		Back:     key.NewBinding(key.WithKeys("q", "esc"), key.WithHelp("q/esc", "back")),
	}
}

type worktreeSelectedMsg struct {
	action   WorktreeAction
	worktree *Worktree
}

type WorktreeList struct {
	list   list.Model
	keyMap worktreeKeyMap
}

func NewWorktreeList(worktrees []*Worktree, width, height int) *WorktreeList {
	items := make([]list.Item, len(worktrees))
	for i, wt := range worktrees {
		items[i] = wt
	}

	keyMap := defaultWorktreeKeyMap()
	l := list.New(items, list.NewDefaultDelegate(), width, height)
	l.Title = "Worktrees"
	l.SetShowHelp(true)
	l.SetShowStatusBar(true)
	l.SetStatusBarItemName("worktree", "worktrees")
	l.AdditionalFullHelpKeys = keyMap.ShortHelp
	if len(worktrees) == 0 {
		l.SetShowFilter(false)
	}

	return &WorktreeList{
		list:   l,
		keyMap: keyMap,
	}
}

func (w *WorktreeList) Init() tea.Cmd {
	return nil
}

func (w *WorktreeList) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, w.keyMap.Review):
			if selected, ok := w.list.SelectedItem().(*Worktree); ok {
				return w, func() tea.Msg {
					return worktreeSelectedMsg{action: ActionReview, worktree: selected}
				}
			}
		case key.Matches(msg, w.keyMap.Interact):
			if selected, ok := w.list.SelectedItem().(*Worktree); ok {
				return w, func() tea.Msg {
					return worktreeSelectedMsg{action: ActionInteract, worktree: selected}
				}
			}
		case key.Matches(msg, w.keyMap.Create):
			return w, func() tea.Msg {
				return worktreeSelectedMsg{action: ActionCreate}
			}
		case key.Matches(msg, w.keyMap.Delete):
			if selected, ok := w.list.SelectedItem().(*Worktree); ok {
				return w, func() tea.Msg {
					return worktreeSelectedMsg{action: ActionDelete, worktree: selected}
				}
			}
		case key.Matches(msg, w.keyMap.Back):
			return w, func() tea.Msg {
				return worktreeSelectedMsg{action: ActionBack}
			}
		}

	case tea.WindowSizeMsg:
		w.list.SetSize(msg.Width, msg.Height)
	}

	var cmd tea.Cmd
	w.list, cmd = w.list.Update(msg)
	return w, cmd
}

func (w *WorktreeList) View() string {
	return w.list.View()
}

func (w *WorktreeList) SetItems(worktrees []*Worktree) {
	items := make([]list.Item, len(worktrees))
	for i, wt := range worktrees {
		items[i] = wt
	}
	w.list.SetItems(items)
}
