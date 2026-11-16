package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/telecom-poc/agent/internal/inventory"
)

func get(t *testing.T, h http.Handler, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestHealthzNoAuth(t *testing.T) {
	h := New("secret", inventory.Inventory{})
	if rec := get(t, h, "/healthz", ""); rec.Code != 200 {
		t.Fatalf("healthz = %d", rec.Code)
	}
}

func TestInventoryNeedsToken(t *testing.T) {
	h := New("secret", inventory.Inventory{Name: "telecom-poc"})
	if rec := get(t, h, "/inventory", ""); rec.Code != 401 {
		t.Fatalf("no token = %d, want 401", rec.Code)
	}
	if rec := get(t, h, "/inventory", "wrong"); rec.Code != 401 {
		t.Fatalf("wrong token = %d, want 401", rec.Code)
	}
	rec := get(t, h, "/inventory", "secret")
	if rec.Code != 200 {
		t.Fatalf("good token = %d", rec.Code)
	}
	var inv inventory.Inventory
	if err := json.Unmarshal(rec.Body.Bytes(), &inv); err != nil || inv.Name != "telecom-poc" {
		t.Fatalf("body %q: %v", rec.Body.String(), err)
	}
}

func TestUnknownRoute404JSON(t *testing.T) {
	rec := get(t, New("secret", inventory.Inventory{}), "/nope", "secret")
	if rec.Code != 404 || rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("got %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
}
