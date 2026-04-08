package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zmb3/spotify/v2"
)

func TestServeConfig(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	w := httptest.NewRecorder()

	serveConfig(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}

	var cfg struct {
		StartYear int `json:"startYear"`
		EndYear   int `json:"endYear"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}

	if cfg.StartYear != 2015 {
		t.Errorf("startYear = %d, want 2015", cfg.StartYear)
	}
	if cfg.EndYear != time.Now().Year() {
		t.Errorf("endYear = %d, want %d", cfg.EndYear, time.Now().Year())
	}
}

func TestServeIndex_NoSession(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	serveIndex(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected redirect 302, got %d", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/login" {
		t.Errorf("Location = %q, want %q", loc, "/login")
	}
}

func TestMakeSongsHandler_NoSession(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/songs?year=2024", nil)
	w := httptest.NewRecorder()

	handler := makeSongsHandler(true)
	handler(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401 for missing session, got %d", w.Code)
	}
}

func TestMakeSongsHandler_MethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/songs?year=2024", nil)
	w := httptest.NewRecorder()

	// Add a valid session so we get past the auth check
	sessionID := generateRandom()
	mu.Lock()
	sessions[sessionID] = spotify.New(http.DefaultClient)
	mu.Unlock()
	req.AddCookie(&http.Cookie{Name: "session", Value: sessionID})

	handler := makeSongsHandler(true)
	handler(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405 for POST, got %d", w.Code)
	}

	mu.Lock()
	delete(sessions, sessionID)
	mu.Unlock()
}
