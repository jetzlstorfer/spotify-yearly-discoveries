package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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

func TestServeIndex(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	serveIndex(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q, want %q", ct, "text/html; charset=utf-8")
	}
}

func TestMakeSongsHandler_MissingYear(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/songs", nil)
	w := httptest.NewRecorder()

	// nil client is safe here because the handler returns before any client call
	handler := makeSongsHandler(nil, true)
	handler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for missing year, got %d", w.Code)
	}
}

func TestMakeSongsHandler_MethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/songs?year=2024", nil)
	w := httptest.NewRecorder()

	handler := makeSongsHandler(nil, true)
	handler(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405 for POST, got %d", w.Code)
	}
}
