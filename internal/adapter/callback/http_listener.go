// Package callback implements the out.CallbackListener driven port with a
// short-lived local HTTP server that captures Spotify's OAuth redirect.
package callback

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"

	"github.com/Emanuel3k/spotify-manager/internal/core/domain"
	"github.com/Emanuel3k/spotify-manager/internal/core/port/out"
)

// Listener binds to the host:port of the CLI's redirect URI and waits for
// exactly one request on its path before shutting itself down.
type Listener struct {
	addr string
	path string
}

var _ out.CallbackListener = (*Listener)(nil)

// New builds a Listener from the configured redirect URI, e.g.
// "http://127.0.0.1:8080/callback". The scheme is ignored: we always bind a
// plain HTTP server locally, since only the browser-to-localhost hop needs
// to work and it never leaves the machine.
func New(redirectURI string) (*Listener, error) {
	u, err := url.Parse(redirectURI)
	if err != nil {
		return nil, fmt.Errorf("parse redirect uri: %w", err)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("redirect uri %q has no host:port", redirectURI)
	}

	path := u.Path
	if path == "" {
		path = "/"
	}

	return &Listener{addr: u.Host, path: path}, nil
}

// Await starts the local server, opens exactly one listener on the redirect
// URI's address, and blocks until a request lands on its path, ctx is
// cancelled, or ctx's deadline is exceeded.
func (l *Listener) Await(ctx context.Context) (out.CallbackResult, error) {
	resultCh := make(chan out.CallbackResult, 1)
	serveErrCh := make(chan error, 1)

	mux := http.NewServeMux()
	srv := &http.Server{Handler: mux}

	mux.HandleFunc(l.path, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		result := out.CallbackResult{
			Code:             q.Get("code"),
			State:            q.Get("state"),
			Error:            q.Get("error"),
			ErrorDescription: q.Get("error_description"),
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		if result.Error != "" {
			_, _ = w.Write([]byte(failureHTML))
		} else {
			_, _ = w.Write([]byte(successHTML))
		}

		select {
		case resultCh <- result:
		default:
			// A stray second request (e.g. browser prefetch) is ignored;
			// the first one already satisfied Await.
		}

		// Shut down after the response is flushed, off the handler
		// goroutine so it doesn't deadlock on itself.
		go func() { _ = srv.Shutdown(context.Background()) }()
	})

	ln, err := net.Listen("tcp", l.addr)
	if err != nil {
		return out.CallbackResult{}, fmt.Errorf("bind callback listener on %s: %w", l.addr, err)
	}

	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			select {
			case serveErrCh <- err:
			default:
			}
		}
	}()
	defer srv.Close()

	select {
	case result := <-resultCh:
		return result, nil
	case err := <-serveErrCh:
		return out.CallbackResult{}, fmt.Errorf("callback server error: %w", err)
	case <-ctx.Done():
		_ = srv.Close()
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return out.CallbackResult{}, domain.ErrLoginTimeout
		}
		return out.CallbackResult{}, ctx.Err()
	}
}

const successHTML = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>Signed in</title>
<style>
body{background:#121212;color:#fff;font-family:-apple-system,Segoe UI,Helvetica,Arial,sans-serif;
display:flex;height:100vh;margin:0;align-items:center;justify-content:center;text-align:center}
.card{padding:2.5rem}
h1{color:#1db954;margin-bottom:.5rem}
p{color:#b3b3b3}
</style></head>
<body><div class="card"><h1>You're signed in</h1><p>spotify-manager is authorized. You can close this tab and go back to the terminal.</p></div></body></html>`

const failureHTML = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>Authorization failed</title>
<style>
body{background:#121212;color:#fff;font-family:-apple-system,Segoe UI,Helvetica,Arial,sans-serif;
display:flex;height:100vh;margin:0;align-items:center;justify-content:center;text-align:center}
.card{padding:2.5rem}
h1{color:#e22134;margin-bottom:.5rem}
p{color:#b3b3b3}
</style></head>
<body><div class="card"><h1>Authorization failed</h1><p>Spotify did not grant access. You can close this tab and check the terminal.</p></div></body></html>`
