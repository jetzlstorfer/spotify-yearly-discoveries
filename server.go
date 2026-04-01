package main

import (
	_ "embed"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"

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

	mux := http.NewServeMux()
	mux.HandleFunc("/", serveIndex)
	mux.HandleFunc("/api/songs", makeSongsHandler(client))

	log.Printf("Web UI running at http://localhost%s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func serveIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(indexHTML)
}

func makeSongsHandler(client spotify.Client) http.HandlerFunc {
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
		tracks := getDiscoveredTracksWithDetails(client, playlists, yr)

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
// and returns full track details (including only saved tracks when ONLY_LOVED_SONGS != "false").
func getDiscoveredTracksWithDetails(client spotify.Client, playlists []spotify.SimplePlaylist, yr string) []TrackInfo {
	seen := make(map[spotify.ID]bool)
	result := []TrackInfo{}
	onlyLoved := os.Getenv("ONLY_LOVED_SONGS") != "false"

	type pending struct {
		id   spotify.ID
		info TrackInfo
	}

	const batchSize = 50

	for _, playlist := range playlists {
		page, err := client.GetPlaylistTracks(playlist.ID)
		if err != nil {
			log.Printf("couldn't get tracks for playlist %q: %v", playlist.Name, err)
			continue
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
			if !strings.Contains(track.Track.Album.ReleaseDate, yr) {
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
	}

	return result
}
