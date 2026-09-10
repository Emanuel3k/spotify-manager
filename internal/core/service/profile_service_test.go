package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Emanuel3k/spotify-manager/internal/core/domain"
	"github.com/Emanuel3k/spotify-manager/internal/core/service"
)

func TestProfileService_Me_Success(t *testing.T) {
	auth := &fakeAuthService{validToken: domain.Token{AccessToken: "at-123"}}
	gw := &fakeProfileGateway{user: domain.User{ID: "u1", DisplayName: "Someone"}}
	svc := service.NewProfileService(auth, gw, nil)

	got, err := svc.Me(context.Background())
	if err != nil {
		t.Fatalf("Me() error = %v, want nil", err)
	}
	if got != gw.user {
		t.Errorf("Me() = %+v, want %+v", got, gw.user)
	}
	if gw.gotToken != "at-123" {
		t.Errorf("CurrentUser called with token %q, want %q", gw.gotToken, "at-123")
	}
}

func TestProfileService_Me_AuthError(t *testing.T) {
	wantErr := errors.New("not authenticated")
	auth := &fakeAuthService{validErr: wantErr}
	gw := &fakeProfileGateway{}
	svc := service.NewProfileService(auth, gw, nil)

	_, err := svc.Me(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("Me() error = %v, want %v", err, wantErr)
	}
	if gw.gotToken != "" {
		t.Errorf("CurrentUser should not have been called when auth fails")
	}
}

func TestProfileService_Me_GatewayError(t *testing.T) {
	wantErr := errors.New("boom")
	auth := &fakeAuthService{validToken: domain.Token{AccessToken: "at"}}
	gw := &fakeProfileGateway{err: wantErr}
	svc := service.NewProfileService(auth, gw, nil)

	_, err := svc.Me(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("Me() error = %v, want wrapped %v", err, wantErr)
	}
}
