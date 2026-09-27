package main

import (
	"path/filepath"
	"testing"
	"time"
)

func TestCredentialsSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials")
	t.Setenv("CORTISOL_CREDENTIALS", path)

	want := sessionCredentials{
		Token: "tok-1", GitHubLogin: "ada", Name: "Ada",
		OrgID: "org-1", OrgName: "Nova",
		ExpiresAt: time.Now().Add(time.Hour).UTC().Truncate(time.Millisecond),
		ServerURL: "http://127.0.0.1:8080",
	}
	if err := saveCredentials(want); err != nil {
		t.Fatal(err)
	}
	got, err := loadCredentials()
	if err != nil {
		t.Fatal(err)
	}
	if got.Token != want.Token || got.GitHubLogin != want.GitHubLogin || got.OrgID != want.OrgID {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestCredentialsExpiredRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials")
	t.Setenv("CORTISOL_CREDENTIALS", path)
	if err := saveCredentials(sessionCredentials{
		Token: "old", ExpiresAt: time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	_, err := loadCredentials()
	if err == nil || err.Error() != "session expired" {
		t.Fatalf("err=%v", err)
	}
}

func TestCredentialsClearRemovesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials")
	t.Setenv("CORTISOL_CREDENTIALS", path)
	if err := saveCredentials(sessionCredentials{
		Token: "tok", ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if err := clearCredentials(); err != nil {
		t.Fatal(err)
	}
	_, err := loadCredentials()
	if err == nil {
		t.Fatal("expected missing credentials after clear")
	}
}
