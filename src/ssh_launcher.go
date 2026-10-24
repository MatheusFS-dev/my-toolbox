package main

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

type sshLauncherEntry struct {
	label    string
	action   string
	disabled bool
}

type sshLauncher struct {
	entries      []sshLauncherEntry
	cursor       int
	width        int
	height       int
	action       string
	availability string
}

func newSSHLauncher(environment, availability string, width, height int) sshLauncher {
	entries := []sshLauncherEntry{{label: "Manage passwordless SSH profiles", action: "profiles"}}
	if environment == "linux-native" {
		for _, entry := range []sshLauncherEntry{
			{label: "Set up direct Ethernet", action: "direct-setup"},
			{label: "Undo direct Ethernet setup", action: "direct-undo"},
			{label: "Install desktop-login status monitor", action: "monitor-install"},
			{label: "Remove desktop-login status monitor", action: "monitor-remove"},
		} {
			entry.disabled = availability != ""
			entries = append(entries, entry)
		}
	}
	return sshLauncher{entries: entries, width: width, height: height, availability: availability}
}

func (model sshLauncher) Init() tea.Cmd { return nil }

func (model sshLauncher) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch event := message.(type) {
	case tea.WindowSizeMsg:
		model.width, model.height = event.Width, event.Height
	case tea.KeyPressMsg:
		if event.Code == 'c' && event.Mod.Contains(tea.ModCtrl) {
			return model, tea.Quit
		}
		switch event.Code {
		case tea.KeyEscape, 'q':
			return model, tea.Quit
		case tea.KeyUp, 'k':
			if model.cursor > 0 {
				model.cursor--
			}
		case tea.KeyDown, 'j':
			if model.cursor+1 < len(model.entries) {
				model.cursor++
			}
		case tea.KeyEnter:
			entry := model.entries[model.cursor]
			if !entry.disabled {
				model.action = entry.action
				return model, tea.Quit
			}
		}
	}
	return model, nil
}

func (model sshLauncher) View() tea.View {
	width := model.width
	if width < 24 {
		width = 24
	}
	lines := []string{"SSH TOOLS", strings.Repeat("─", min(width, 72)), "Choose an action"}
	for index, entry := range model.entries {
		marker := "  "
		if index == model.cursor {
			marker = "› "
		}
		label := entry.label
		if entry.disabled {
			label += " (unavailable)"
		}
		lines = append(lines, marker+sshFit(label, width-2))
	}
	if model.availability != "" {
		lines = append(lines, "", sshFit(model.availability, width))
	}
	lines = append(lines, "", "↑/↓ select  •  enter choose  •  q quit")
	if model.height > 0 && len(lines) > model.height {
		lines = lines[:model.height]
	}
	view := tea.NewView(strings.Join(lines, "\n"))
	view.AltScreen = true
	return view
}
