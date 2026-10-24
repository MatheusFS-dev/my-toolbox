package main

import (
	"fmt"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

type sshDashboard struct {
	profiles []SSHProfile
	cursor   int
	width    int
	height   int
	action   string
	alias    string
}

func newSSHDashboard(profiles []SSHProfile, _ string, width, height int) sshDashboard {
	return sshDashboard{profiles: profiles, width: width, height: height}
}

func (model sshDashboard) Init() tea.Cmd { return nil }

func (model sshDashboard) Update(message tea.Msg) (tea.Model, tea.Cmd) {
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
			if model.cursor+1 < len(model.profiles) {
				model.cursor++
			}
		case 'a':
			model.action = "add"
		case 'r':
			model.action = "refresh"
		case 't', 'e', 'x', tea.KeyEnter:
			if len(model.profiles) > 0 {
				model.alias = model.profiles[model.cursor].Alias
				switch event.Code {
				case 't':
					model.action = "test"
				case 'e':
					model.action = "edit"
				case 'x':
					model.action = "remove"
				default:
					model.action = "connect"
				}
			}
		}
		if model.action != "" {
			return model, tea.Quit
		}
	}
	return model, nil
}

func (model sshDashboard) View() tea.View {
	width := model.width
	if width < 24 {
		width = 24
	}
	var lines []string
	lines = append(lines, "SSH TOOLS", strings.Repeat("─", min(width, 72)))
	profileLines := []string{"Saved profiles"}
	if len(model.profiles) == 0 {
		profileLines = append(profileLines, "No saved connections. Press a to add one.")
	}
	for index, profile := range model.profiles {
		marker := "  "
		if index == model.cursor {
			marker = "› "
		}
		row := fmt.Sprintf("%s%s  %s@%s:%d  %s", marker, profile.Alias, profile.User, profile.Hostname, profile.Port, profile.Health)
		profileLines = append(profileLines, row)
	}
	lines = append(lines, profileLines...)
	lines = append(lines, "", "↑/↓ select  •  enter connect  •  a add  •  e edit  •  t test  •  x remove", "r refresh  •  q back")
	if model.height > 0 && len(lines) > model.height {
		lines = lines[:model.height]
	}
	return tea.NewView(strings.Join(lines, "\n"))
}

func sshFit(value string, width int) string {
	if width < 1 {
		return ""
	}
	if utf8.RuneCountInString(value) <= width {
		return value
	}
	return string([]rune(value)[:width-1]) + "…"
}
