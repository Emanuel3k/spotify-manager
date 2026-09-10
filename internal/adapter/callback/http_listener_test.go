package callback_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Emanuel3k/spotify-manager/internal/adapter/callback"
	"github.com/Emanuel3k/spotify-manager/internal/core/domain"
	"github.com/Emanuel3k/spotify-manager/internal/core/port/out"
)

// freePort asks the OS for an ephemeral port and immediately releases it,
// so the redirect URI used to build the Listener is valid up front.
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find a free port: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// getWithRetry polls url until it gets a response or ctx is done: the
// listener's net.Listen happens inside Await, running in a goroutine the
// test just started, so the port may not be bound yet on the first try.
func getWithRetry(ctx context.Context, url string) (*http.Response, error) {
	var lastErr error
	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("giving up after retries: %w", lastErr)
		default:
		}
		resp, err := http.Get(url)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		time.Sleep(10 * time.Millisecond)
	}
}

func TestListener_Await_Success(t *testing.T) {
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", freePort(t))
	l, err := callback.New(redirectURI)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	type awaitOutcome struct {
		result out.CallbackResult
		err    error
	}
	resultCh := make(chan awaitOutcome, 1)
	go func() {
		result, err := l.Await(context.Background())
		resultCh <- awaitOutcome{result, err}
	}()

	reqCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	resp, err := getWithRetry(reqCtx, redirectURI+"?code=abc123&state=xyz789")
	if err != nil {
		t.Fatalf("GET callback: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("response status = %d, want 200", resp.StatusCode)
	}
	if !strings.Contains(string(body), "signed in") {
		t.Errorf("response body = %q, want it to mention being signed in", body)
	}

	select {
	case outcome := <-resultCh:
		if outcome.err != nil {
			t.Fatalf("Await() error = %v, want nil", outcome.err)
		}
		if outcome.result.Code != "abc123" || outcome.result.State != "xyz789" {
			t.Errorf("Await() result = %+v, want Code=abc123 State=xyz789", outcome.result)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Await() did not return after the callback request")
	}
}

func TestListener_Await_SpotifyError(t *testing.T) {
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", freePort(t))
	l, err := callback.New(redirectURI)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	resultCh := make(chan out.CallbackResult, 1)
	go func() {
		result, _ := l.Await(context.Background())
		resultCh <- result
	}()

	reqCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	resp, err := getWithRetry(reqCtx, redirectURI+"?error=access_denied&error_description=nope&state=xyz")
	if err != nil {
		t.Fatalf("GET callback: %v", err)
	}
	resp.Body.Close()

	select {
	case result := <-resultCh:
		if result.Error != "access_denied" {
			t.Errorf("result.Error = %q, want %q", result.Error, "access_denied")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Await() did not return after the callback request")
	}
}

func TestListener_Await_Timeout(t *testing.T) {
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", freePort(t))
	l, err := callback.New(redirectURI)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err = l.Await(ctx)
	if !errors.Is(err, domain.ErrLoginTimeout) {
		t.Fatalf("Await() error = %v, want ErrLoginTimeout", err)
	}
}

func TestNew_InvalidRedirectURI(t *testing.T) {
	if _, err := callback.New("not-a-url-with-a-host"); err == nil {
		t.Fatal("New() error = nil, want an error for a redirect uri without a host")
	}
}
