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

// maxVisibleMenuItems caps how many items menuModel draws at once. Without
// this, a list built from real data (e.g. the hundreds of distinct artists
// in a large playlist) rendered every item on every keypress — overflowing
// the terminal and making the menu feel broken/laggy. A fixed-size
// scrolling window keeps rendering cost constant regardless of list size.
const maxVisibleMenuItems = 12

// menuModel is a minimal bubbletea program: an arrow-key list with a single
// selection, windowed to maxVisibleMenuItems. bubbletea (unlike the
// readline-based prompt libraries) drives the terminal through proper
// raw-mode handling on Windows, so arrow keys don't trigger the console
// error beep. It's reused by every arrow-key picker in this package (see
// pickFromList), not just the top-level menu.
type menuModel struct {
	title       string
	items       []string
	cursor      int
	windowStart int
	chosen      int // -1 while running; set on Enter (index) or quit (-2)
}

func newMenuModel(title string, items []string) menuModel {
	return menuModel{title: title, items: items, chosen: -1}
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
	case "pgup":
		m.cursor -= maxVisibleMenuItems
		if m.cursor < 0 {
			m.cursor = 0
		}
	case "pgdown":
		m.cursor += maxVisibleMenuItems
		if m.cursor > len(m.items)-1 {
			m.cursor = len(m.items) - 1
		}
	case "enter":
		m.chosen = m.cursor
		return m, tea.Quit
	}

	// Keep the cursor inside the visible window, scrolling it as needed.
	if m.cursor < m.windowStart {
		m.windowStart = m.cursor
	}
	if m.cursor >= m.windowStart+maxVisibleMenuItems {
		m.windowStart = m.cursor - maxVisibleMenuItems + 1
	}
	return m, nil
}

func (m menuModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(m.title) + "\n\n")

	end := min(m.windowStart+maxVisibleMenuItems, len(m.items))
	if m.windowStart > 0 {
		b.WriteString(hintStyle.Render("  ↑ mais acima") + "\n")
	}
	for i := m.windowStart; i < end; i++ {
		if i == m.cursor {
			b.WriteString(cursorStyle.Render("❱ ") + selectedStyle.Render(m.items[i]) + "\n")
		} else {
			b.WriteString("  " + m.items[i] + "\n")
		}
	}
	if end < len(m.items) {
		b.WriteString(hintStyle.Render("  ↓ mais abaixo") + "\n")
	}

	hint := fmt.Sprintf("↑/↓ navega · enter confirma · q sai  (%d/%d)", m.cursor+1, len(m.items))
	b.WriteString("\n" + hintStyle.Render(hint))
	return boxStyle.Render(b.String())
}

// pickFromList runs an arrow-key picker over items and returns the chosen
// index. ok is false if the user quit (q/Esc/Ctrl+C) instead of picking.
func pickFromList(title string, items []string) (idx int, ok bool, err error) {
	program := tea.NewProgram(newMenuModel(title, items))
	finalModel, err := program.Run()
	if err != nil {
		return 0, false, fmt.Errorf("menu: %w", err)
	}

	chosen := finalModel.(menuModel).chosen
	if chosen < 0 {
		return 0, false, nil
	}
	return chosen, true, nil
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
		{"Criar playlist a partir de um artista", runInteractiveArtistPlaylist},
		{"Sair", nil},
	}

	items := make([]string, len(actions))
	for i, a := range actions {
		items[i] = a.label
	}

	for {
		chosen, ok, err := pickFromList("spotify-manager", items)
		if err != nil {
			return err
		}
		if !ok {
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
