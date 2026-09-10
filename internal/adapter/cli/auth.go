package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
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
		fmt.Fprintln(out, "Opening your browser to log in with Spotify...")
		fmt.Fprintln(out, "If it doesn't open automatically, visit this URL:")
		fmt.Fprintln(out, "  "+authURL)
	})
	if err != nil {
		return mapLoginErr(err)
	}

	fmt.Fprintln(out, "Logged in successfully.")
	fmt.Fprintf(out, "Granted scopes: %s\n", token.Scope)

	if deps.Profile != nil {
		if user, err := deps.Profile.Me(ctx); err == nil {
			printUser(out, user)
		}
		// A failure here doesn't invalidate the login itself (the token is
		// already stored), so it's silently skipped: the user can still
		// confirm with `auth whoami`.
	}

	return nil
}

func runAuthWhoami(ctx context.Context, deps Deps, out io.Writer) error {
	user, err := deps.Profile.Me(ctx)
	if err != nil {
		if errors.Is(err, domain.ErrNotAuthenticated) {
			return errors.New("not logged in: run `spotify-manager auth login`")
		}
		return err
	}
	printUser(out, user)
	return nil
}

func runAuthLogout(ctx context.Context, deps Deps, out io.Writer) error {
	if err := deps.Auth.Logout(ctx); err != nil {
		return err
	}
	fmt.Fprintln(out, "Logged out.")
	return nil
}

func runAuthStatus(ctx context.Context, deps Deps, out io.Writer) error {
	status, err := deps.Auth.Status(ctx)
	if err != nil {
		return err
	}

	if !status.Authenticated {
		fmt.Fprintln(out, "Not logged in. Run `spotify-manager auth login`.")
		return nil
	}

	fmt.Fprintln(out, "Logged in.")
	fmt.Fprintf(out, "Scopes: %s\n", status.Scope)
	fmt.Fprintf(out, "Access token expires at: %s\n", status.ExpiresAt)
	return nil
}

func printUser(out io.Writer, user domain.User) {
	fmt.Fprintf(out, "Logged in as: %s (id: %s)\n", user.DisplayName, user.ID)
	if user.Email != "" {
		fmt.Fprintf(out, "Email: %s\n", user.Email)
	}
	if user.Product != "" {
		fmt.Fprintf(out, "Plan: %s\n", user.Product)
	}
}

// mapLoginErr turns core sentinel errors into short, user-facing messages
// while leaving any other error's wrapped chain intact for debugging.
func mapLoginErr(err error) error {
	switch {
	case errors.Is(err, domain.ErrAuthorizationDenied):
		return errors.New("login cancelled: access was denied in the browser")
	case errors.Is(err, domain.ErrLoginTimeout):
		return fmt.Errorf("login timed out after %s waiting for browser authorization", loginTimeout)
	case errors.Is(err, domain.ErrStateMismatch):
		return errors.New("login aborted: oauth state did not match (possible tampering), please try again")
	default:
		return err
	}
}
