package main

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
)

//go:embed web/index.html
var indexHTML []byte

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

var (
	sessions    = map[string]*spotify.Client{}
	oauthStates = map[string]bool{}
	mu          sync.Mutex
)

func generateRandom() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("could not generate random bytes: %v", err)
	}
	return hex.EncodeToString(b)
}

func getClient(r *http.Request) *spotify.Client {
	cookie, err := r.Cookie("session")
	if err != nil {
		return nil
	}
	mu.Lock()
	defer mu.Unlock()
	return sessions[cookie.Value]
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

	mux := http.NewServeMux()
	mux.HandleFunc("/", serveIndex)
	mux.HandleFunc("/login", handleLogin)
	mux.HandleFunc("/logout", handleLogout)
	mux.HandleFunc("/callback", handleCallback)
	mux.HandleFunc("/api/config", serveConfig)
	mux.HandleFunc("/api/songs", makeSongsHandler(onlyLoved))

	log.Printf("Web UI running at http://127.0.0.1%s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
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
		log.Printf("token error: %v", err)
		return
	}

	httpClient := spotifyauth.New().Client(r.Context(), tok)
	client := spotify.New(httpClient)

	sessionID := generateRandom()
	mu.Lock()
	sessions[sessionID] = client
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
	w.Write(indexHTML)
}

// serveConfig returns UI configuration (start/end year) so the frontend
// does not need hard-coded values.
func serveConfig(w http.ResponseWriter, r *http.Request) {
	type config struct {
		StartYear int `json:"startYear"`
		EndYear   int `json:"endYear"`
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config{StartYear: 2015, EndYear: time.Now().Year()})
}

func makeSongsHandler(onlyLoved bool) http.HandlerFunc {
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

		ctx := r.Context()
		playlists := getPlaylistsForYear(ctx, client, yr)
		tracks := getDiscoveredTracksWithDetails(ctx, client, playlists, yr, onlyLoved)

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(tracks); err != nil {
			log.Printf("error encoding response: %v", err)
		}
	}
}

// getPlaylistsForYear returns all user playlists whose name contains the given year.
func getPlaylistsForYear(ctx context.Context, client *spotify.Client, yr string) []spotify.SimplePlaylist {
	var result []spotify.SimplePlaylist
	offset := 0
	limit := 50

	for p := 1; ; p++ {
		page, err := client.CurrentUsersPlaylists(ctx, spotify.Limit(limit), spotify.Offset(offset))
		if err != nil {
			log.Printf("couldn't get playlists: %v", err)
			break
		}
		for _, pl := range page.Playlists {
			if strings.Contains(pl.Name, yr) {
				result = append(result, pl)
			}
		}
		offset = p * limit
		if page.Next == "" {
			break
		}
	}
	return result
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
	pageLimit := 100

	for _, playlist := range playlists {
		var offset int

		for {
			page, err := client.GetPlaylistTracks(ctx, playlist.ID, spotify.Limit(pageLimit), spotify.Offset(offset))
			if err != nil {
				log.Printf("couldn't get tracks for playlist %q: %v", playlist.Name, err)
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
					log.Printf("couldn't check library status: %v", err)
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
				if !strings.HasPrefix(track.Track.Album.ReleaseDate, yr) {
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
