package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/jetzlstorfer/spotify-yearly-discoveries/internal/randutil"
	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
)

//go:embed web/index.html
var indexHTML []byte

//go:embed web/tailwind.min.css
var tailwindCSS []byte

// TrackInfo holds the song details returned to the UI.
type TrackInfo struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Artists     []string `json:"artists"`
	Album       string   `json:"album"`
	ReleaseDate string   `json:"releaseDate"`
	AlbumArtURL string   `json:"albumArtUrl"`
	SpotifyURL  string   `json:"spotifyUrl"`
	DurationMs  int      `json:"durationMs"`
	Playlist    string   `json:"playlist"`
}

// sessionEntry holds a Spotify client together with its expiry time so that
// the background cleanup goroutine can discard stale sessions.
type sessionEntry struct {
	client    *spotify.Client
	expiresAt time.Time
}

var (
	sessions    = map[string]*sessionEntry{}
	oauthStates = map[string]bool{}
	mu          sync.Mutex
)

// cleanExpiredSessions removes all sessions whose TTL has elapsed.
func cleanExpiredSessions() {
	mu.Lock()
	defer mu.Unlock()
	now := time.Now()
	for id, s := range sessions {
		if now.After(s.expiresAt) {
			delete(sessions, id)
		}
	}
}

func generateRandom() string {
	return randutil.HexString(16)
}

func getClient(r *http.Request) *spotify.Client {
	cookie, err := r.Cookie("session")
	if err != nil {
		return nil
	}
	mu.Lock()
	defer mu.Unlock()
	entry, ok := sessions[cookie.Value]
	if !ok || time.Now().After(entry.expiresAt) {
		delete(sessions, cookie.Value)
		return nil
	}
	return entry.client
}

func buildRedirectURI(r *http.Request) string {
	if uri := os.Getenv("REDIRECT_URI"); uri != "" {
		return uri
	}
	scheme := "https"
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	} else if r.TLS == nil {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s/callback", scheme, r.Host)
}

func newAuthenticator(r *http.Request) *spotifyauth.Authenticator {
	a := spotifyauth.New(
		spotifyauth.WithRedirectURL(buildRedirectURI(r)),
		spotifyauth.WithScopes(
			spotifyauth.ScopePlaylistReadPrivate,
			spotifyauth.ScopePlaylistModifyPrivate,
			spotifyauth.ScopePlaylistModifyPublic,
			spotifyauth.ScopeUserLibraryRead,
		),
	)
	return a
}

func startWebServer(addr string) {
	onlyLoved, err := strconv.ParseBool(os.Getenv("ONLY_LOVED_SONGS"))
	if err != nil {
		onlyLoved = true
	}

	cache := newResultsCache()

	// Background goroutine to evict expired sessions.
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			cleanExpiredSessions()
		}
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("/", serveIndex)
	mux.HandleFunc("/login", handleLogin)
	mux.HandleFunc("/logout", handleLogout)
	mux.HandleFunc("/callback", handleCallback)
	mux.HandleFunc("/api/config", serveConfig)
	mux.HandleFunc("/api/songs", makeSongsHandler(onlyLoved, cache))
	mux.HandleFunc("/tailwind.min.css", serveTailwindCSS)

	slog.Info("Web UI running", "addr", "http://127.0.0.1"+addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}

func serveTailwindCSS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	if _, err := w.Write(tailwindCSS); err != nil {
		slog.Error("error writing tailwind CSS response", "err", err)
	}
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("session"); err == nil {
		mu.Lock()
		delete(sessions, cookie.Value)
		mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
	http.Redirect(w, r, "/login", http.StatusFound)
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	state := generateRandom()
	mu.Lock()
	oauthStates[state] = true
	mu.Unlock()

	auth := newAuthenticator(r)
	http.Redirect(w, r, auth.AuthURL(state), http.StatusFound)
}

func handleCallback(w http.ResponseWriter, r *http.Request) {
	state := r.FormValue("state")
	mu.Lock()
	valid := oauthStates[state]
	delete(oauthStates, state)
	mu.Unlock()

	if !valid {
		http.Error(w, "invalid state", http.StatusForbidden)
		return
	}

	auth := newAuthenticator(r)
	tok, err := auth.Token(r.Context(), state, r)
	if err != nil {
		http.Error(w, "could not get token", http.StatusForbidden)
		slog.Error("token error", "err", err)
		return
	}

	httpClient := spotifyauth.New().Client(r.Context(), tok)
	client := spotify.New(httpClient)

	sessionID := generateRandom()
	mu.Lock()
	sessions[sessionID] = &sessionEntry{
		client:    client,
		expiresAt: time.Now().Add(time.Hour),
	}
	mu.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   3600,
	})

	http.Redirect(w, r, "/", http.StatusFound)
}

func serveIndex(w http.ResponseWriter, r *http.Request) {
	if getClient(r) == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := w.Write(indexHTML); err != nil {
		slog.Error("error writing index response", "err", err)
	}
}

// serveConfig returns UI configuration so the frontend does not need
// hard-coded values. START_YEAR can be set via the environment variable
// of the same name; it defaults to 2015.
func serveConfig(w http.ResponseWriter, r *http.Request) {
	type config struct {
		StartYear int `json:"startYear"`
		EndYear   int `json:"endYear"`
	}

	startYear := 2015
	if s := os.Getenv("START_YEAR"); s != "" {
		if y, err := strconv.Atoi(s); err == nil && y > 1900 && y <= time.Now().Year() {
			startYear = y
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(config{StartYear: startYear, EndYear: time.Now().Year()}); err != nil {
		slog.Error("error encoding config response", "err", err)
	}
}

// resultsCache is a simple TTL-based in-memory cache for /api/songs results,
// keyed by "<userID>:<year>". This avoids repeated full Spotify API scans when
// a user clicks the same year button multiple times.
type cacheEntry struct {
	tracks    []TrackInfo
	expiresAt time.Time
}

type resultsCache struct {
	mu      sync.Mutex
	entries map[string]*cacheEntry
}

func newResultsCache() *resultsCache {
	c := &resultsCache{entries: make(map[string]*cacheEntry)}
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			c.purge()
		}
	}()
	return c
}

func (c *resultsCache) get(key string) ([]TrackInfo, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok || time.Now().After(e.expiresAt) {
		delete(c.entries, key)
		return nil, false
	}
	return e.tracks, true
}

func (c *resultsCache) set(key string, tracks []TrackInfo, ttl time.Duration) {
	if len(tracks) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = &cacheEntry{tracks: tracks, expiresAt: time.Now().Add(ttl)}
}

func (c *resultsCache) purge() {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	for k, e := range c.entries {
		if now.After(e.expiresAt) {
			delete(c.entries, k)
		}
	}
}

func makeSongsHandler(onlyLoved bool, cache *resultsCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		client := getClient(r)
		if client == nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		yr := r.URL.Query().Get("year")
		if yr == "" {
			http.Error(w, "year parameter is required", http.StatusBadRequest)
			return
		}

		// Impose a per-request deadline so a slow Spotify API cannot block
		// the handler goroutine indefinitely.
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
		defer cancel()

		currentUser, err := client.CurrentUser(ctx)
		if err != nil {
			http.Error(w, "could not get current user", http.StatusInternalServerError)
			slog.Error("could not get current user", "err", err)
			return
		}

		cacheKey := currentUser.ID + ":" + yr
		if cached, ok := cache.get(cacheKey); ok {
			slog.Info("serving songs from cache", "user", currentUser.ID, "year", yr)
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(cached); err != nil {
				slog.Error("error encoding cached response", "err", err)
			}
			return
		}

		playlists := fetchPlaylistsForYear(ctx, client, yr, currentUser.ID, "")
		tracks := getDiscoveredTracksWithDetails(ctx, client, playlists, yr, onlyLoved)

		cache.set(cacheKey, tracks, 10*time.Minute)

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(tracks); err != nil {
			slog.Error("error encoding response", "err", err)
		}
	}
}

// getDiscoveredTracksWithDetails scans playlists for tracks released in the given year
// and returns full track details (including only saved tracks when onlyLoved is true).
// Playlists with more than 100 tracks are fully paginated.
func getDiscoveredTracksWithDetails(ctx context.Context, client *spotify.Client, playlists []spotify.SimplePlaylist, yr string, onlyLoved bool) []TrackInfo {
	seen := make(map[spotify.ID]bool)
	result := []TrackInfo{}

	type pending struct {
		id   spotify.ID
		info TrackInfo
	}

	const batchSize = 50

	for _, playlist := range playlists {
		var offset int

		for {
			page, err := client.GetPlaylistTracks(ctx, playlist.ID, spotify.Limit(pageLimit), spotify.Offset(offset))
			if err != nil {
				slog.Error("couldn't get tracks for playlist", "playlist", playlist.Name, "err", err)
				break
			}

			var batch []pending

			flush := func() {
				if len(batch) == 0 {
					return
				}
				ids := make([]spotify.ID, len(batch))
				for i, p := range batch {
					ids[i] = p.id
				}
				isAdded, err := client.UserHasTracks(ctx, ids...)
				if err != nil {
					slog.Error("couldn't check library status", "err", err)
					for _, p := range batch {
						if !seen[p.id] {
							seen[p.id] = true
							result = append(result, p.info)
						}
					}
					batch = nil
					return
				}
				for i, added := range isAdded {
					if added || !onlyLoved {
						if !seen[batch[i].id] {
							seen[batch[i].id] = true
							result = append(result, batch[i].info)
						}
					}
				}
				batch = nil
			}

			for _, track := range page.Tracks {
				// Use HasPrefix so "2025" matches "2025-03-15" but not a date
				// that merely contains "2025" as a substring elsewhere.
				if !trackIsFromYear(track, yr) {
					continue
				}

				artists := make([]string, len(track.Track.Artists))
				for i, a := range track.Track.Artists {
					artists[i] = a.Name
				}

				artURL := ""
				if len(track.Track.Album.Images) > 0 {
					artURL = track.Track.Album.Images[0].URL
				}

				info := TrackInfo{
					ID:          string(track.Track.ID),
					Name:        track.Track.Name,
					Artists:     artists,
					Album:       track.Track.Album.Name,
					ReleaseDate: track.Track.Album.ReleaseDate,
					AlbumArtURL: artURL,
					SpotifyURL:  track.Track.ExternalURLs["spotify"],
					DurationMs:  int(track.Track.Duration),
					Playlist:    playlist.Name,
				}

				batch = append(batch, pending{id: track.Track.ID, info: info})
				if len(batch) >= batchSize {
					flush()
				}
			}
			flush()

			offset += pageLimit
			if page.Next == "" {
				break
			}
		}
	}

	return result
}
