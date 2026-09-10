// Package tokenstore implements the out.TokenRepository driven port as a
// JSON file on disk, under the OS's per-user config directory.
package tokenstore

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Emanuel3k/spotify-manager/internal/core/domain"
	"github.com/Emanuel3k/spotify-manager/internal/core/port/out"
)

// dirPerm/filePerm keep the credentials file readable only by the owner.
// On Windows these bits are mostly advisory (ACLs govern real access), but
// they are still the right POSIX-side intent for a cross-platform tool.
const (
	dirPerm  os.FileMode = 0o700
	filePerm os.FileMode = 0o600
)

// FileStore persists a single Token as JSON at <configDir>/spotify-manager/credentials.json.
type FileStore struct {
	path string
}

var _ out.TokenRepository = (*FileStore)(nil)

// NewFileStore builds a FileStore rooted at the OS user config directory.
func NewFileStore() (*FileStore, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("resolve user config dir: %w", err)
	}
	return &FileStore{path: filepath.Join(base, "spotify-manager", "credentials.json")}, nil
}

// NewFileStoreAt builds a FileStore at an explicit path, mainly for tests.
func NewFileStoreAt(path string) *FileStore {
	return &FileStore{path: path}
}

// storedToken mirrors domain.Token for JSON (de)serialization, keeping the
// domain type itself free of struct tags / persistence concerns.
type storedToken struct {
	AccessToken  string    `json:"access_token"`
	TokenType    string    `json:"token_type"`
	Scope        string    `json:"scope"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

func (s *FileStore) Save(_ context.Context, token domain.Token) error {
	if err := os.MkdirAll(filepath.Dir(s.path), dirPerm); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(storedToken{
		AccessToken:  token.AccessToken,
		TokenType:    token.TokenType,
		Scope:        token.Scope,
		RefreshToken: token.RefreshToken,
		ExpiresAt:    token.ExpiresAt,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal token: %w", err)
	}

	if err := os.WriteFile(s.path, data, filePerm); err != nil {
		return fmt.Errorf("write credentials file: %w", err)
	}
	return nil
}

func (s *FileStore) Load(_ context.Context) (domain.Token, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return domain.Token{}, fmt.Errorf("%w", domain.ErrNotAuthenticated)
		}
		return domain.Token{}, fmt.Errorf("read credentials file: %w", err)
	}

	var st storedToken
	if err := json.Unmarshal(data, &st); err != nil {
		return domain.Token{}, fmt.Errorf("parse credentials file: %w", err)
	}

	return domain.Token{
		AccessToken:  st.AccessToken,
		TokenType:    st.TokenType,
		Scope:        st.Scope,
		RefreshToken: st.RefreshToken,
		ExpiresAt:    st.ExpiresAt,
	}, nil
}

func (s *FileStore) Delete(_ context.Context) error {
	if err := os.Remove(s.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove credentials file: %w", err)
	}
	return nil
}
