package spotifyweb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

const defaultBaseURL = "https://api.spotify.com"

// client is the shared low-level HTTP plumbing for every Web API gateway in
// this package (profile, playlists, ...): it centralizes auth headers,
// error-body parsing and request logging so each gateway only deals with
// its own endpoints and JSON shapes.
type client struct {
	httpClient *http.Client
	baseURL    string
	log        *slog.Logger
}

func newClient(logger *slog.Logger) client {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return client{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		baseURL:    defaultBaseURL,
		log:        logger.With("component", "spotifyweb_client"),
	}
}

// webAPIError mirrors the Web API's { "error": { "status": ..., "message": ... } }
// body returned on non-2xx responses.
type webAPIError struct {
	Error struct {
		Status  int    `json:"status"`
		Message string `json:"message"`
	} `json:"error"`
}

// do performs an authenticated request against baseURL+path, decoding a
// JSON response body into out (when out is non-nil and the response has a
// body). op is a short label used only for log correlation.
func (c *client) do(ctx context.Context, op, method, path, accessToken string, body io.Reader, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return fmt.Errorf("build %s request: %w", op, err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	start := time.Now()
	c.log.Debug("web api request", "op", op, "method", method, "path", path)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.log.Error("web api call failed", "op", op, "method", method, "path", path, "error", err)
		return fmt.Errorf("call %s: %w", op, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.log.Error("web api response read failed", "op", op, "error", err)
		return fmt.Errorf("read %s response: %w", op, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiErr webAPIError
		_ = json.Unmarshal(respBody, &apiErr)
		c.log.Error("web api returned an error", "op", op, "method", method, "path", path, "status", resp.Status, "message", apiErr.Error.Message)
		if apiErr.Error.Message != "" {
			return fmt.Errorf("spotify web api %s %s returned %s: %s", method, path, resp.Status, apiErr.Error.Message)
		}
		return fmt.Errorf("spotify web api %s %s returned %s: %s", method, path, resp.Status, string(respBody))
	}

	c.log.Debug("web api request succeeded", "op", op, "method", method, "path", path, "status", resp.StatusCode, "duration_ms", time.Since(start).Milliseconds())

	if out == nil || len(respBody) == 0 {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		c.log.Error("web api response decode failed", "op", op, "error", err)
		return fmt.Errorf("decode %s response: %w", op, err)
	}
	return nil
}
