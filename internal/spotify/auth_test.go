package spotify

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAuthStorage(t *testing.T) {
	dir := t.TempDir()
	am := NewAuthManager(dir, "test-client-id")
	creds := Credentials{
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
		ExpiresAt:    time.Now().Add(1 * time.Hour),
		Product:      "premium",
	}
	if err := am.Save(creds); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := am.Credentials()
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.AccessToken != creds.AccessToken || got.Product != "premium" {
		t.Errorf("got %+v, want %+v", got, creds)
	}
}

func TestProactiveTokenRefresh(t *testing.T) {
	dir := t.TempDir()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/token" {
			_ = r.ParseForm()
			if r.Form.Get("grant_type") != "refresh_token" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "new-refreshed-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	am := NewAuthManager(dir, "test-client-id")
	am.tokenURL = ts.URL + "/api/token"

	// Guardar un token que caduca en 1 minuto (menos del umbral proactivo de 5 minutos)
	creds := Credentials{
		AccessToken:  "old-token",
		RefreshToken: "valid-refresh-token",
		ExpiresAt:    time.Now().Add(1 * time.Minute),
		Product:      "premium",
	}
	if err := am.Save(creds); err != nil {
		t.Fatalf("save: %v", err)
	}

	token, err := am.ProactiveToken()
	if err != nil {
		t.Fatalf("ProactiveToken error: %v", err)
	}
	if token != "new-refreshed-token" {
		t.Errorf("expected new-refreshed-token, got %s", token)
	}

	updated, err := am.Credentials()
	if err != nil {
		t.Fatalf("credentials: %v", err)
	}
	if updated.AccessToken != "new-refreshed-token" {
		t.Errorf("expected updated token to be stored, got %s", updated.AccessToken)
	}
}
