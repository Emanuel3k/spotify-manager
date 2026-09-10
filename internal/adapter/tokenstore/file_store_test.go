package tokenstore_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Emanuel3k/spotify-manager/internal/adapter/tokenstore"
	"github.com/Emanuel3k/spotify-manager/internal/core/domain"
)

func TestFileStore_SaveLoadDelete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "credentials.json")
	store := tokenstore.NewFileStoreAt(path)
	ctx := context.Background()

	if _, err := store.Load(ctx); !errors.Is(err, domain.ErrNotAuthenticated) {
		t.Fatalf("Load() before Save() error = %v, want ErrNotAuthenticated", err)
	}

	want := domain.Token{
		AccessToken:  "at",
		TokenType:    "Bearer",
		Scope:        "user-read-email",
		RefreshToken: "rt",
		ExpiresAt:    time.Now().Add(time.Hour).Truncate(time.Second),
	}
	if err := store.Save(ctx, want); err != nil {
		t.Fatalf("Save() error = %v, want nil", err)
	}

	got, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if !got.ExpiresAt.Equal(want.ExpiresAt) || got.AccessToken != want.AccessToken ||
		got.RefreshToken != want.RefreshToken || got.Scope != want.Scope || got.TokenType != want.TokenType {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}

	if err := store.Delete(ctx); err != nil {
		t.Fatalf("Delete() error = %v, want nil", err)
	}
	if _, err := store.Load(ctx); !errors.Is(err, domain.ErrNotAuthenticated) {
		t.Fatalf("Load() after Delete() error = %v, want ErrNotAuthenticated", err)
	}

	// Delete must be idempotent.
	if err := store.Delete(ctx); err != nil {
		t.Fatalf("second Delete() error = %v, want nil", err)
	}
}
