package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"golang.org/x/oauth2"
)

var year = os.Getenv("YEAR_TO_CHECK")
var logFilePath = os.Getenv("LOG_FILE")
var tokenFile = os.Getenv("TOKEN_FILE")
var yearsPlaylistID = os.Getenv("PLAYLIST_ID")

func main() {
	webMode := flag.Bool("web", false, "start the web UI instead of running the batch job")
	port := flag.String("port", "8080", "port for the web server (only used with -web)")
	flag.Parse()

	if *webMode {
		startWebServer(":" + *port)
		return
	}

	slog.Info("Starting program...")

	if yearsPlaylistID == "" {
		slog.Error("environment variable PLAYLIST_ID is empty, can not create playlist")
		os.Exit(1)
	}

	if year == "" {
		slog.Error("environment variable YEAR_TO_CHECK is empty, can not proceed")
		os.Exit(1)
	}

	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0666)
	if err != nil {
		slog.Error("could not open log file", "err", err)
		os.Exit(1)
	}
	mw := io.MultiWriter(os.Stdout, logFile)
	slog.SetDefault(slog.New(slog.NewTextHandler(mw, nil)))

	// ONLY_LOVED_SONGS accepts standard boolean strings: true/false, 1/0, TRUE/FALSE.
	// Defaults to true (only include tracks saved to the user's library) if unset or invalid.
	onlyLoved, err := strconv.ParseBool(os.Getenv("ONLY_LOVED_SONGS"))
	if err != nil {
		onlyLoved = true
	}

	ctx := context.Background()
	client, err := verifyLogin(ctx)
	if err != nil {
		slog.Error("could not verify login", "err", err)
		os.Exit(1)
	}

	// get current user to filter owned playlists only
	currentUser, err := client.CurrentUser(ctx)
	if err != nil {
		slog.Error("could not get current user", "err", err)
		os.Exit(1)
	}

	// get all playlists that match the YEAR in the playlist title, excluding the target playlist
	playlistsToConsider := fetchPlaylistsForYear(ctx, client, year, currentUser.ID, yearsPlaylistID)
	slog.Info("playlists found", "year", year, "count", len(playlistsToConsider))

	yearlyDiscovery := getDiscoveredSongsFromPlaylists(ctx, client, playlistsToConsider, onlyLoved)

	slog.Info("songs discovered", "count", len(yearlyDiscovery))

	// removing duplicate values
	yearlyDiscovery = removeDuplicateValues(yearlyDiscovery)
	slog.Info("songs discovered (unique)", "count", len(yearlyDiscovery))

	// empty playlist
	if err = client.ReplacePlaylistTracks(ctx, spotify.ID(yearsPlaylistID)); err != nil {
		slog.Error("could not empty playlist", "err", err)
		os.Exit(1)
	}

	// add tracks
	slog.Info("adding discovered songs to playlist")
	if err = addTracksToPlaylist(ctx, client, yearlyDiscovery); err != nil {
		slog.Error("could not add songs to playlist", "err", err)
		os.Exit(1)
	}

	updatedPlaylist, err := client.GetPlaylist(ctx, spotify.ID(yearsPlaylistID))
	if err != nil {
		slog.Error("could not get updated playlist", "err", err)
		os.Exit(1)
	}
	slog.Info("songs added to playlist", "count", updatedPlaylist.Tracks.Total)
}

const pageLimit = 100

func getDiscoveredSongsFromPlaylists(ctx context.Context, client *spotify.Client, playlistsToConsider []spotify.SimplePlaylist, onlyLoved bool) []spotify.ID {
	var yearlyDiscovery []spotify.ID
	trackLimit := 50

	for _, playlist := range playlistsToConsider {
		var offset int

		for {
			page, err := client.GetPlaylistTracks(ctx, playlist.ID, spotify.Limit(pageLimit), spotify.Offset(offset))
			if err != nil {
				slog.Error("couldn't get tracks for playlist", "playlist", playlist.Name, "err", err)
				os.Exit(1)
			}

			var tracksToCheck []spotify.ID
			for _, track := range page.Tracks {
				// check if track is from YEAR using HasPrefix so "2025" matches
				// "2025-03-15" but not a year that merely contains "2025" elsewhere.
				if trackIsFromYear(track, year) {
					slog.Info("song matching year found",
						"year", year,
						"artist", track.Track.Artists[0].Name,
						"track", track.Track.Name,
					)

					tracksToCheck = append(tracksToCheck, track.Track.ID)
					slog.Debug("tracks pending library check", "count", len(tracksToCheck))

					// if trackLimit is reached, check all songs if they have been added to library
					if len(tracksToCheck) >= trackLimit {
						yearlyDiscovery = append(yearlyDiscovery, getAddedTracks(ctx, client, tracksToCheck, onlyLoved)...)
						tracksToCheck = nil
					}
				}
			}
			// flush remaining tracks before moving to the next page/playlist
			yearlyDiscovery = append(yearlyDiscovery, getAddedTracks(ctx, client, tracksToCheck, onlyLoved)...)

			offset += pageLimit
			if page.Next == "" {
				break
			}
		}
	}
	return yearlyDiscovery
}

// addTracksToPlaylist adds songs to the target playlist, 100 tracks per API call.
func addTracksToPlaylist(ctx context.Context, client *spotify.Client, tracks []spotify.ID) error {
	var limit = 100
	for i := 0; i < len(tracks); i += limit {
		batch := tracks[i:min(i+limit, len(tracks))]
		if _, err := client.AddTracksToPlaylist(ctx, spotify.ID(yearsPlaylistID), batch...); err != nil {
			return fmt.Errorf("could not add songs to playlist: %w", err)
		}
	}
	return nil
}

// trackIsFromYear reports whether the track's album was released in yr.
// It uses HasPrefix so "2025" matches "2025-03-15" but not, say, "12025".
func trackIsFromYear(track spotify.PlaylistTrack, yr string) bool {
	return strings.HasPrefix(track.Track.Album.ReleaseDate, yr)
}

func removeDuplicateValues(slice []spotify.ID) []spotify.ID {
	keys := make(map[spotify.ID]struct{})
	list := []spotify.ID{}

	for _, entry := range slice {
		if _, value := keys[entry]; !value {
			keys[entry] = struct{}{}
			list = append(list, entry)
		}
	}
	return list
}

// getAddedTracks filters tracksToCheck to only those saved to the user's
// library (when onlyLoved is true). onlyLoved should be resolved once at
// startup rather than re-reading the environment on every call.
func getAddedTracks(ctx context.Context, client *spotify.Client, tracksToCheck []spotify.ID, onlyLoved bool) []spotify.ID {
	if len(tracksToCheck) == 0 {
		return nil
	}

	slog.Debug("checking library status for tracks", "count", len(tracksToCheck))

	isAdded, err := client.UserHasTracks(ctx, tracksToCheck...)
	if err != nil {
		slog.Error("could not check library status", "err", err)
		os.Exit(1)
	}

	var yearlyDiscovery []spotify.ID
	for j, added := range isAdded {
		if added || !onlyLoved {
			yearlyDiscovery = append(yearlyDiscovery, tracksToCheck[j])
		}
	}
	return yearlyDiscovery
}

func verifyLogin(ctx context.Context) (*spotify.Client, error) {
	data, err := os.ReadFile(tokenFile)
	if err != nil {
		return nil, fmt.Errorf("could not read token file: %w", err)
	}

	tok := new(oauth2.Token)
	if err := json.Unmarshal(data, tok); err != nil {
		return nil, fmt.Errorf("could not unmarshal token: %w", err)
	}

	// Create a Spotify authenticator with the oauth2 token.
	// If the token is expired, the oauth2 package will automatically refresh;
	// the new token is then saved so future runs use the refreshed credentials.
	httpClient := spotifyauth.New().Client(ctx, tok)
	client := spotify.New(httpClient)

	newToken, err := client.Token()
	if err != nil {
		return nil, fmt.Errorf("could not retrieve token from client: %w", err)
	}
	if newToken.AccessToken != tok.AccessToken {
		slog.Info("got refreshed token, saving it")
		tokenBytes, err := json.Marshal(newToken)
		if err != nil {
			return nil, fmt.Errorf("could not marshal token: %w", err)
		}
		if err := os.WriteFile(tokenFile, tokenBytes, 0600); err != nil {
			return nil, fmt.Errorf("could not save refreshed token: %w", err)
		}
	}

	user, err := client.CurrentUser(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not get current user: %w", err)
	}
	slog.Info("logged in", "user", user.ID)

	return client, nil
}
