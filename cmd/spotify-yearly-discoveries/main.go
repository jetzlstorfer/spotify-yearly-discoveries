package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/zmb3/spotify"
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

	log.Println("Starting program...")

	if yearsPlaylistID == "" {
		log.Fatalln("environment variable PLAYLIST_ID is empty, can not create playlist.")
	}

	if year == "" {
		log.Fatalln("environment variable YEAR_TO_CHECK is empty, can not proceed.")
	}

	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0666)
	if err != nil {
		log.Fatalln(err)
	}
	mw := io.MultiWriter(os.Stdout, logFile)
	log.SetOutput(mw)

	client, err := verifyLogin()
	if err != nil {
		log.Fatalln("could not verify login: " + err.Error())
	}

	// get all playlists that match the YEAR in the playlist title
	playlistsToConsider := getPlaylistsMatchingCondition(client, year)
	log.Println("Number of playlists with " + year + " in title: " + strconv.Itoa(len(playlistsToConsider)))

	yearlyDiscovery := getDiscoveredSongsFromPlaylists(client, playlistsToConsider)

	log.Println("Songs discovered: " + strconv.Itoa(len(yearlyDiscovery)))

	// removing duplicate values
	yearlyDiscovery = removeDuplicateValues(yearlyDiscovery)
	log.Println("Songs discovered (unique): " + strconv.Itoa(len(yearlyDiscovery)))

	// empty playlist
	if err = client.ReplacePlaylistTracks(spotify.ID(yearsPlaylistID)); err != nil {
		log.Fatalln("could not empty playlist: " + err.Error())
	}

	// add tracks
	log.Println("adding discovered songs to playlist...")
	if err = addTracksToPlaylist(client, yearlyDiscovery); err != nil {
		log.Fatalln("Could not add songs to playlist: " + err.Error())
	}

	updatedPlaylist, err := client.GetPlaylist(spotify.ID(yearsPlaylistID))
	if err != nil {
		log.Fatalln(err)
	}
	log.Println(strconv.Itoa(updatedPlaylist.Tracks.Total) + " songs added to new playlist")

}

func getPlaylistsMatchingCondition(client spotify.Client, condition string) []spotify.SimplePlaylist {
	var offset int = 0
	var limit int = 50
	var playlistsToConsider []spotify.SimplePlaylist
	for p := 1; ; p++ {

		opt := spotify.Options{Limit: &limit, Offset: &offset}

		page, err := client.CurrentUsersPlaylistsOpt(&opt)
		if err != nil {
			log.Fatalf("couldn't get playlists: %v", err)
		}

		for _, playlist := range page.Playlists {
			if strings.Contains(playlist.Name, condition) && playlist.ID != spotify.ID(yearsPlaylistID) {
				playlistsToConsider = append(playlistsToConsider, playlist)
			}
		}
		offset = p * limit
		if page.Next == "" {
			break
		}
	}
	return playlistsToConsider
}

func getDiscoveredSongsFromPlaylists(client spotify.Client, playlistsToConsider []spotify.SimplePlaylist) []spotify.ID {
	var yearlyDiscovery []spotify.ID
	trackLimit := 50
	pageLimit := 100

	for _, playlist := range playlistsToConsider {
		var offset int

		for {
			opt := spotify.Options{Limit: &pageLimit, Offset: &offset}
			page, err := client.GetPlaylistTracksOpt(playlist.ID, &opt, "")
			if err != nil {
				log.Fatalf("couldn't get tracks for playlist %s: %v", playlist.Name, err)
			}

			var tracksToCheck []spotify.ID
			for _, track := range page.Tracks {
				// check if track is from YEAR
				if trackIsFromYear(track) {

					log.Println("Song matching " + year + " found: " + track.Track.Artists[0].Name + " - " + track.Track.Name)

					tracksToCheck = append(tracksToCheck, track.Track.ID)
					log.Println("size of collection of tracks to check: " + strconv.Itoa(len(tracksToCheck)))

					// if trackLimit is reached, check all songs if they have been added to library and add them to list
					if len(tracksToCheck) >= trackLimit {
						yearlyDiscovery = append(yearlyDiscovery, getAddedTracks(client, tracksToCheck)...)
						tracksToCheck = nil
					}
				}
			}
			// flush remaining tracks before moving to the next page/playlist
			yearlyDiscovery = append(yearlyDiscovery, getAddedTracks(client, tracksToCheck)...)

			offset += pageLimit
			if page.Next == "" {
				break
			}
		}
	}
	return yearlyDiscovery
}

// adding songs to playlist, 100 songs per API call
func addTracksToPlaylist(client spotify.Client, tracks []spotify.ID) error {
	var limit = 100
	for i := 0; i < len(tracks); i += limit {
		batch := tracks[i:min(i+limit, len(tracks))]
		if _, err := client.AddTracksToPlaylist(spotify.ID(yearsPlaylistID), batch...); err != nil {
			return fmt.Errorf("could not add songs to playlist: %w", err)
		}
	}
	return nil
}

func trackIsFromYear(track spotify.PlaylistTrack) bool {
	// log.Println("ReleaseDate: " + track.Track.Album.ReleaseDate)
	return strings.Contains(track.Track.Album.ReleaseDate, year)
	// return false
}

func removeDuplicateValues(slice []spotify.ID) []spotify.ID {
	keys := make(map[spotify.ID]bool)
	list := []spotify.ID{}

	// If the key(values of the slice) is not equal
	// to the already present value in new slice (list)
	// then we append it. else we jump on another element.
	for _, entry := range slice {
		if _, value := keys[entry]; !value {
			keys[entry] = true
			list = append(list, entry)
		}
	}
	return list
}

func getAddedTracks(client spotify.Client, tracksToCheck []spotify.ID) []spotify.ID {
	var yearlyDiscovery []spotify.ID
	// check if song is added to users library
	log.Println("size of collection of tracks to check that is added: " + strconv.Itoa(len(tracksToCheck)))
	if len(tracksToCheck) == 0 {
		return nil
	}

	// ONLY_LOVED_SONGS accepts standard boolean strings: true/false, 1/0, TRUE/FALSE.
	// Defaults to true (only include tracks saved to the user's library) if unset or invalid.
	onlyLovedSongs, err := strconv.ParseBool(os.Getenv("ONLY_LOVED_SONGS"))
	if err != nil {
		onlyLovedSongs = true // default to only loved songs
	}

	isAdded, err := client.UserHasTracks(tracksToCheck...)
	if err != nil {
		log.Fatalln(err)
	}
	for j, added := range isAdded {
		if added || !onlyLovedSongs {
			yearlyDiscovery = append(yearlyDiscovery, tracksToCheck[j])
		}
	}
	return yearlyDiscovery
}

func verifyLogin() (spotify.Client, error) {

	data, err := os.ReadFile(tokenFile)
	if err != nil {
		return spotify.Client{}, fmt.Errorf("could not read token file: %w", err)
	}

	tok := new(oauth2.Token)
	if err := json.Unmarshal(data, tok); err != nil {
		return spotify.Client{}, fmt.Errorf("could not unmarshal token: %w", err)
	}

	// Create a Spotify authenticator with the oauth2 token.
	// If the token is expired, the oauth2 package will automatically refresh
	// so the new token is checked against the old one to see if it should be updated.
	client := spotify.NewAuthenticator("").NewClient(tok)

	newToken, err := client.Token()
	if err != nil {
		return spotify.Client{}, fmt.Errorf("could not retrieve token from client: %w", err)
	}
	if newToken.AccessToken != tok.AccessToken {
		log.Println("got refreshed token, saving it")
		tokenBytes, err := json.Marshal(newToken)
		if err != nil {
			return spotify.Client{}, fmt.Errorf("could not marshal token: %w", err)
		}
		if err := os.WriteFile(tokenFile, tokenBytes, 0600); err != nil {
			return spotify.Client{}, fmt.Errorf("could not save refreshed token: %w", err)
		}
	}

	// use the client to make calls that require authorization
	user, err := client.CurrentUser()
	if err != nil {
		return spotify.Client{}, fmt.Errorf("could not get current user: %w", err)
	}
	log.Println("You are logged in as: ", user.ID)

	return client, nil
}
