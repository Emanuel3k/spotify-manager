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

// menuAction is one leaf entry of an action menu (as opposed to a category
// that opens a submenu). run reuses the exact same functions the
// scriptable subcommands call (see auth.go, playlist.go) so the two
// interfaces never drift apart. run == nil means "go back" (a "Voltar"
// entry, or the top-level "Sair").
type menuAction struct {
	label string
	run   func(ctx context.Context, deps Deps, out io.Writer) error
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

	hint := fmt.Sprintf("↑/↓ navega · enter confirma · q volta  (%d/%d)", m.cursor+1, len(m.items))
	b.WriteString("\n" + hintStyle.Render(hint))
	return boxStyle.Render(b.String())
}

// pickFromList runs an arrow-key picker over items and returns the chosen
// index. ok is false if the user backed out (q/Esc/Ctrl+C) instead of
// picking.
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

// Top-level category labels and every menu item label across this file
// follow the same style on purpose (part of a "clean, standardized,
// friendly" pass): no emoji anywhere, every action phrased as an imperative
// verb ("Entrar", "Criar playlist por ano", "Voltar"), consistent
// capitalization. Keep new entries consistent with this when extending the
// menu — don't mix an emoji-prefixed label in among plain ones, or a
// noun-phrase label in among verb-phrase ones.
const (
	labelAccount   = "Conta"
	labelPlaylists = "Playlists"
	labelExit      = "Sair"
)

// runInteractiveMenu is the root command's default behavior: launched when
// the binary is run with no subcommand. It's a two-layer menu — a top-level
// picker over categories ("Conta", "Playlists") that each open their own
// submenu — so day-to-day use doesn't require remembering flags. Account
// comes first: it's the natural starting point (who am I, am I logged in)
// before doing anything with playlists. The panel title shows the
// logged-in account's name once known (fetched lazily, only right after
// the auth state actually changes, not on every redraw).
func runInteractiveMenu(ctx context.Context, deps Deps, out io.Writer) error {
	var authKnown, authed bool
	var accountName string

	for {
		status, _ := deps.Auth.Status(ctx)
		if !authKnown || status.Authenticated != authed {
			authKnown = true
			authed = status.Authenticated
			accountName = ""
			if authed {
				if user, err := deps.Profile.Me(ctx); err == nil {
					accountName = user.DisplayName
				}
			}
		}

		title := "spotify-manager"
		if accountName != "" {
			title = fmt.Sprintf("spotify-manager · %s", accountName)
		}

		idx, ok, err := pickFromList(title, []string{labelAccount, labelPlaylists, labelExit})
		if err != nil {
			return err
		}
		if !ok {
			fmt.Fprintln(out, "Até mais!")
			return nil
		}

		switch idx {
		case 0:
			if err := runAccountMenu(ctx, deps, out); err != nil {
				return err
			}
		case 1:
			if err := runPlaylistsMenu(ctx, deps, out); err != nil {
				return err
			}
		case 2:
			fmt.Fprintln(out, "Até mais!")
			return nil
		}
	}
}

// runPlaylistsMenu is the "Playlists" category submenu. Its items don't
// depend on runtime state, so it's just a static action menu.
func runPlaylistsMenu(ctx context.Context, deps Deps, out io.Writer) error {
	return runActionMenu(ctx, deps, out, labelPlaylists, []menuAction{
		{"Dividir playlist por ano", runInteractivePlaylistSplit},
		{"Criar playlist por artista", runInteractiveArtistPlaylist},
		{"Voltar", nil},
	})
}

// runAccountMenu is the "Conta" category submenu. Unlike runPlaylistsMenu
// it rebuilds its item list every loop iteration, since it must show
// exactly one of "Entrar"/"Sair da conta" depending on current session
// state — e.g. right after a successful login it must immediately switch
// to offering to log out, not keep offering to log in.
func runAccountMenu(ctx context.Context, deps Deps, out io.Writer) error {
	for {
		status, _ := deps.Auth.Status(ctx)

		actions := []menuAction{{"Ver status da conta", runAuthStatus}}
		if status.Authenticated {
			actions = append(actions, menuAction{"Sair da conta", runAuthLogout})
		} else {
			actions = append(actions, menuAction{"Entrar", runAuthLogin})
		}
		actions = append(actions, menuAction{"Voltar", nil})

		items := make([]string, len(actions))
		for i, a := range actions {
			items[i] = a.label
		}

		idx, ok, err := pickFromList(labelAccount, items)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}

		action := actions[idx]
		if action.run == nil {
			return nil
		}

		fmt.Fprintln(out)
		if err := action.run(ctx, deps, out); err != nil {
			fmt.Fprintln(out, "Erro:", err)
		}
		waitForEnter(out)
	}
}

// runActionMenu loops a static leaf-action menu until the user picks
// "Voltar" (run == nil) or backs out (q/Esc/Ctrl+C) — both return to the
// caller (the parent menu), not the whole program.
func runActionMenu(ctx context.Context, deps Deps, out io.Writer, title string, actions []menuAction) error {
	items := make([]string, len(actions))
	for i, a := range actions {
		items[i] = a.label
	}

	for {
		idx, ok, err := pickFromList(title, items)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}

		action := actions[idx]
		if action.run == nil {
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
