package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// menuAction is one entry of the interactive menu. run reuses the exact
// same functions the scriptable subcommands call (see auth.go, playlist.go)
// so the two interfaces never drift apart.
type menuAction struct {
	label string
	run   func(ctx context.Context, deps Deps, out io.Writer) error // nil for "exit"
}

var (
	boxStyle      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(1, 3)
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	cursorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	hintStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

// menuModel is a minimal bubbletea program: an arrow-key list with a single
// selection. bubbletea (unlike the readline-based prompt libraries) drives
// the terminal through proper raw-mode handling on Windows, so arrow keys
// don't trigger the console error beep.
type menuModel struct {
	items  []string
	cursor int
	chosen int // -1 while running; set on Enter (index) or quit (-2)
}

func newMenuModel(items []string) menuModel {
	return menuModel{items: items, chosen: -1}
}

func (m menuModel) Init() tea.Cmd { return nil }

func (m menuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch keyMsg.String() {
	case "ctrl+c", "q", "esc":
		m.chosen = -2
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.items)-1 {
			m.cursor++
		}
	case "enter":
		m.chosen = m.cursor
		return m, tea.Quit
	}
	return m, nil
}

func (m menuModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("spotify-manager") + "\n\n")

	for i, item := range m.items {
		if i == m.cursor {
			b.WriteString(cursorStyle.Render("❱ ") + selectedStyle.Render(item) + "\n")
		} else {
			b.WriteString("  " + item + "\n")
		}
	}

	b.WriteString("\n" + hintStyle.Render("↑/↓ navega · enter confirma · q sai"))
	return boxStyle.Render(b.String())
}

// runInteractiveMenu is the root command's default behavior: launched when
// the binary is run with no subcommand, it loops an arrow-key menu until
// the user picks "Sair" or quits (q / Esc / Ctrl+C).
func runInteractiveMenu(ctx context.Context, deps Deps, out io.Writer) error {
	actions := []menuAction{
		{"Login", runAuthLogin},
		{"Status da conta", runAuthStatus},
		{"Quem sou eu (whoami)", runAuthWhoami},
		{"Logout", runAuthLogout},
		{"Dividir playlist por ano", runInteractivePlaylistSplit},
		{"Sair", nil},
	}

	items := make([]string, len(actions))
	for i, a := range actions {
		items[i] = a.label
	}

	for {
		program := tea.NewProgram(newMenuModel(items))
		finalModel, err := program.Run()
		if err != nil {
			return fmt.Errorf("menu: %w", err)
		}

		chosen := finalModel.(menuModel).chosen
		if chosen < 0 {
			fmt.Fprintln(out, "Até mais!")
			return nil
		}

		action := actions[chosen]
		if action.run == nil {
			fmt.Fprintln(out, "Até mais!")
			return nil
		}

		fmt.Fprintln(out)
		if err := action.run(ctx, deps, out); err != nil {
			fmt.Fprintln(out, "Erro:", err)
		}
		waitForEnter(out)
	}
}

// runInteractivePlaylistSplit prompts for the playlist link (the one piece
// of input `playlist split-by-year` takes as a CLI arg) and then defers to
// the same logic the subcommand uses. Plain line-based stdin is enough here
// (no arrow-key navigation needed), so it doesn't need bubbletea/raw mode.
func runInteractivePlaylistSplit(ctx context.Context, deps Deps, out io.Writer) error {
	fmt.Fprint(out, "Link da playlist: ")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && err != io.EOF {
		return err
	}

	link := strings.TrimSpace(line)
	if link == "" {
		fmt.Fprintln(out, "Nenhum link informado.")
		return nil
	}

	return runPlaylistSplitByYear(ctx, deps, out, link)
}

func waitForEnter(out io.Writer) {
	fmt.Fprint(out, "\nPressione Enter para voltar ao menu...")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
}
