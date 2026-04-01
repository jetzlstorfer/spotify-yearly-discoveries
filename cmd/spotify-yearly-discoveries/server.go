package main

import (
	_ "embed"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/zmb3/spotify"
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

func startWebServer(addr string) {
	client, err := verifyLogin()
	if err != nil {
		log.Fatalf("login failed: %v", err)
	}

	// ONLY_LOVED_SONGS accepts standard boolean strings: true/false, 1/0, TRUE/FALSE.
	// Defaults to true (only include tracks saved to the user's library) if unset or invalid.
	onlyLoved, err := strconv.ParseBool(os.Getenv("ONLY_LOVED_SONGS"))
	if err != nil {
		onlyLoved = true
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", serveIndex)
	mux.HandleFunc("/api/config", serveConfig)
	mux.HandleFunc("/api/songs", makeSongsHandler(client, onlyLoved))

	log.Printf("Web UI running at http://localhost%s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func serveIndex(w http.ResponseWriter, r *http.Request) {
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

func makeSongsHandler(client spotify.Client, onlyLoved bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		yr := r.URL.Query().Get("year")
		if yr == "" {
			http.Error(w, "year parameter is required", http.StatusBadRequest)
			return
		}

		playlists := getPlaylistsForYear(client, yr)
		tracks := getDiscoveredTracksWithDetails(client, playlists, yr, onlyLoved)

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(tracks); err != nil {
			log.Printf("error encoding response: %v", err)
		}
	}
}

// getPlaylistsForYear returns all user playlists whose name contains the given year.
func getPlaylistsForYear(client spotify.Client, yr string) []spotify.SimplePlaylist {
	var result []spotify.SimplePlaylist
	offset := 0
	limit := 50

	for p := 1; ; p++ {
		opt := spotify.Options{Limit: &limit, Offset: &offset}
		page, err := client.CurrentUsersPlaylistsOpt(&opt)
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
func getDiscoveredTracksWithDetails(client spotify.Client, playlists []spotify.SimplePlaylist, yr string, onlyLoved bool) []TrackInfo {
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
			opt := spotify.Options{Limit: &pageLimit, Offset: &offset}
			page, err := client.GetPlaylistTracksOpt(playlist.ID, &opt, "")
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
				isAdded, err := client.UserHasTracks(ids...)
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
					DurationMs:  track.Track.Duration,
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
