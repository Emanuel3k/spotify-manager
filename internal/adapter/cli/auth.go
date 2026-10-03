package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Emanuel3k/spotify-manager/internal/core/domain"
)

// loginTimeout bounds how long `auth login` waits for the user to finish
// the consent screen in their browser before giving up.
const loginTimeout = 5 * time.Minute

func newAuthCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage Spotify authentication",
	}

	cmd.AddCommand(
		newAuthLoginCmd(deps),
		newAuthLogoutCmd(deps),
		newAuthStatusCmd(deps),
		newAuthWhoamiCmd(deps),
	)

	return cmd
}

func newAuthLoginCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Log in to Spotify through your browser",
		Long: "Starts the Spotify Authorization Code flow: opens your default browser to\n" +
			"Spotify's consent screen and waits for it to redirect back to a local\n" +
			"server, then stores the resulting access/refresh token for future commands.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAuthLogin(cmd.Context(), deps, cmd.OutOrStdout())
		},
	}
}

func newAuthWhoamiCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show the Spotify account currently logged in",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAuthWhoami(cmd.Context(), deps, cmd.OutOrStdout())
		},
	}
}

func newAuthLogoutCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Forget the stored Spotify session",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAuthLogout(cmd.Context(), deps, cmd.OutOrStdout())
		},
	}
}

func newAuthStatusCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show whether you're currently logged in",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAuthStatus(cmd.Context(), deps, cmd.OutOrStdout())
		},
	}
}

// runAuthLogin, runAuthWhoami, runAuthLogout and runAuthStatus hold the
// actual command logic, decoupled from cobra so the interactive menu
// (internal/adapter/cli/interactive.go) can call the exact same behavior
// the scriptable subcommands use.

func runAuthLogin(ctx context.Context, deps Deps, out io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, loginTimeout)
	defer cancel()

	token, err := deps.Auth.Login(ctx, func(authURL string) {
		fmt.Fprintln(out, "Abrindo o navegador para autorizar o acesso à sua conta Spotify...")
		fmt.Fprintln(out, "Se não abrir sozinho, acesse este link:")
		fmt.Fprintln(out, "  "+authURL)
	})
	if err != nil {
		return mapLoginErr(err)
	}

	var user *domain.User
	if deps.Profile != nil {
		if u, err := deps.Profile.Me(ctx); err == nil {
			user = &u
		}
		// A failure here doesn't invalidate the login itself (the token is
		// already stored), so it's silently skipped: the user can still
		// confirm with `auth whoami` or `auth status`.
	}

	renderAccountPanel(out, "Login realizado", token.Scope, token.ExpiresAt.Format(displayTimeLayout), user)
	return nil
}

func runAuthWhoami(ctx context.Context, deps Deps, out io.Writer) error {
	user, err := deps.Profile.Me(ctx)
	if err != nil {
		if errors.Is(err, domain.ErrNotAuthenticated) {
			return errors.New("você ainda não entrou na conta: rode `spotify-manager auth login`")
		}
		return err
	}
	renderAccountPanel(out, "Conta", "", "", &user)
	return nil
}

func runAuthLogout(ctx context.Context, deps Deps, out io.Writer) error {
	if err := deps.Auth.Logout(ctx); err != nil {
		return err
	}
	fmt.Fprintln(out, "Você saiu da conta.")
	return nil
}

func runAuthStatus(ctx context.Context, deps Deps, out io.Writer) error {
	status, err := deps.Auth.Status(ctx)
	if err != nil {
		return err
	}

	if !status.Authenticated {
		renderInfoPanel(out, "Status da conta", "Você ainda não entrou na sua conta Spotify.",
			"Use \"Entrar\" no menu Conta, ou rode: spotify-manager auth login")
		return nil
	}

	var user *domain.User
	if deps.Profile != nil {
		if u, err := deps.Profile.Me(ctx); err == nil {
			user = &u
		}
	}

	expiresAt := status.ExpiresAt
	if t, err := time.Parse(time.RFC3339, status.ExpiresAt); err == nil {
		expiresAt = t.Format(displayTimeLayout)
	}

	renderAccountPanel(out, "Status da conta", status.Scope, expiresAt, user)
	return nil
}

// displayTimeLayout is the human-friendly date/time format used across the
// account panels (Brazilian dd/mm/yyyy, 24h clock).
const displayTimeLayout = "02/01/2006 15:04"

// renderAccountPanel draws the standardized "account info" box reused by
// login, whoami and status — the same boxStyle/titleStyle/hintStyle as the
// interactive menu (interactive.go), so every account-related screen reads
// as one consistent UI. scope/expiresAt/user are each optional (pass ""
// or nil to omit that section) so the same renderer fits whoami (identity
// only), login (identity + fresh token info) and status (token info, with
// identity if available).
func renderAccountPanel(out io.Writer, title, scope, expiresAt string, user *domain.User) {
	var b strings.Builder

	if user != nil {
		if user.DisplayName != "" {
			b.WriteString(user.DisplayName + "\n")
		}
		if user.Email != "" {
			b.WriteString(hintStyle.Render(user.Email) + "\n")
		}
		if user.DisplayName != "" || user.Email != "" {
			b.WriteString("\n")
		}
	}

	type row struct{ label, value string }
	var rows []row
	if user != nil && user.Product != "" {
		rows = append(rows, row{"Plano", strings.ToUpper(user.Product[:1]) + user.Product[1:]})
	}
	if expiresAt != "" {
		rows = append(rows, row{"Expira em", expiresAt})
	}
	if scope != "" {
		rows = append(rows, row{"Permissões", fmt.Sprintf("%d concedidas", len(strings.Fields(scope)))})
	}

	labelWidth := 0
	for _, r := range rows {
		labelWidth = max(labelWidth, len([]rune(r.label)))
	}
	for _, r := range rows {
		b.WriteString(hintStyle.Render(fmt.Sprintf("%-*s", labelWidth, r.label)) + "   " + r.value + "\n")
	}

	content := strings.TrimRight(b.String(), "\n")
	fmt.Fprintln(out, boxStyle.Render(titleStyle.Render(title)+"\n\n"+content))
}

// renderInfoPanel draws the same standardized box for a plain message (no
// account fields), e.g. the "not logged in" state.
func renderInfoPanel(out io.Writer, title string, lines ...string) {
	body := strings.Join(lines, "\n")
	fmt.Fprintln(out, boxStyle.Render(titleStyle.Render(title)+"\n\n"+hintStyle.Render(body)))
}

// mapLoginErr turns core sentinel errors into short, user-facing messages
// while leaving any other error's wrapped chain intact for debugging.
func mapLoginErr(err error) error {
	switch {
	case errors.Is(err, domain.ErrAuthorizationDenied):
		return errors.New("login cancelado: o acesso foi negado no navegador")
	case errors.Is(err, domain.ErrLoginTimeout):
		return fmt.Errorf("login expirou após %s esperando a autorização no navegador", loginTimeout)
	case errors.Is(err, domain.ErrStateMismatch):
		return errors.New("login abortado: o state do oauth não bateu (possível adulteração), tente novamente")
	default:
		return err
	}
}
