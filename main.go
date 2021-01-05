package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/zmb3/spotify"
	"golang.org/x/oauth2"
)

const redirectURI = "http://localhost:8888/callback"

var year = os.Getenv("YEAR_TO_CHECK")
var logFile = os.Getenv("LOG_FILE")
var tokenFile = os.Getenv("TOKEN_FILE")
var yearsPlaylistID = os.Getenv("PLAYLIST_ID")

var (
	auth  = spotify.NewAuthenticator(redirectURI, spotify.ScopePlaylistReadPrivate)
	ch    = make(chan *spotify.Client)
	state = "abc123"
)

func main() {

	log.Println("Starting program...")

	if os.Getenv("PLAYLIST_ID") == "" {
		log.Fatalln("environment variable PLAYLIST_ID is empty, can not create playlist.")
	}

	logFile, err := os.OpenFile(logFile, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0666)
	if err != nil {
		log.Fatalln(err)
	}
	mw := io.MultiWriter(os.Stdout, logFile)
	log.SetOutput(mw)

	client, _ := verifyLogin()

	// get all playlists that match the YEAR in the playlist title
	playlistsToConsider := getPlaylistsMatchingCondition(client, year)
	log.Println("Number of playlists with " + year + " in title: " + strconv.Itoa(len(playlistsToConsider)))

	// todo
	yearlyDiscovery := getDiscoveredSongsFromPlaylists(client, playlistsToConsider)

	log.Println("Songs discovered: " + strconv.Itoa(len(yearlyDiscovery)))

	// removing duplicate values
	yearlyDiscovery = removeDuplicateValues(yearlyDiscovery)
	log.Println("Songs discovered (unique): " + strconv.Itoa(len(yearlyDiscovery)))

	// empty playlist
	client.ReplacePlaylistTracks(spotify.ID(yearsPlaylistID))
	if err != nil {
		log.Fatalln("could not empty playlist: " + err.Error())
	}

	// add tracks
	log.Println("adding discovered songs to playlist...")
	err = addTracksToPlaylist(client, yearlyDiscovery)
	if err != nil {
		log.Fatalln("Could not add songs to playlist: " + err.Error())
	}

	updatedPlaylist, err := client.GetPlaylist(spotify.ID(yearsPlaylistID))
	if err != nil {
		log.Fatalln(err)
	}
	log.Println(strconv.Itoa((updatedPlaylist.Tracks.Total)) + " songs added new playlist")

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
			if page.Next != "" {

			}
			// log.Println(playlist.Name)
			if strings.Contains(playlist.Name, year) && playlist.ID != spotify.ID(yearsPlaylistID) {
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

	for _, playlist := range playlistsToConsider {
		//	log.Println("Scaning playlist: " + playlist.Name)
		page, _ := client.GetPlaylistTracks(playlist.ID)

		var tracksToCheck []spotify.ID
		for i, track := range page.Tracks {
			// check if track is from YEAR
			if trackIsFromYear((track)) {

				log.Println("Song matching " + year + " found: " + track.Track.Artists[0].Name + " - " + track.Track.Name)

				tracksToCheck = append(tracksToCheck, track.Track.ID)
				log.Println("size of collection of tracks to check: " + strconv.Itoa(len(tracksToCheck)))

				// if trackLimit is reached, check all songs if they have been added to library and add them to list
				if len(tracksToCheck) >= trackLimit {
					yearlyDiscovery = append(yearlyDiscovery, getAddedTracks(client, tracksToCheck)...)
					tracksToCheck = nil
				}

			}
			// at the end before checking the next playlist, add all songs to the list
			if i+1 >= len(page.Tracks) {
				// add songs that have been added to users library to the list
				yearlyDiscovery = append(yearlyDiscovery, getAddedTracks(client, tracksToCheck)...)
				tracksToCheck = nil
			}

		}

	}
	return yearlyDiscovery
}

// adding songs to playlist, 100 songs each Api call
func addTracksToPlaylist(client spotify.Client, tracks []spotify.ID) error {
	var limit = 100
	for i := 0; i < len(tracks); i += limit {
		tracks := tracks[i:min(i+limit, len(tracks))]
		_, err := client.AddTracksToPlaylist(spotify.ID(yearsPlaylistID), tracks...)
		if err != nil {
			log.Fatalln("Could not add songs to playlist: " + err.Error())
			return err
		}
	}
	return nil
}

// helpfer function
func min(a, b int) int {
	if a <= b {
		return a
	}
	return b
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
	isAdded, err := client.UserHasTracks(tracksToCheck...)
	if err != nil {
		log.Fatalln(err)
	}
	for j, added := range isAdded {

		if added {
			// log.Println("song was saved to users library, saving it to collection")
			// save it to be added to yearly discovery playlist
			yearlyDiscovery = append(yearlyDiscovery, tracksToCheck[j])
		} else {
			// log.Println("song not saved to users library, skipping it")
		}
	}
	return yearlyDiscovery
}

func verifyLogin() (spotify.Client, error) {

	file, err := os.Open(tokenFile)
	if err != nil {
		fmt.Println(err)
		// return
	}
	defer file.Close()

	fileinfo, err := file.Stat()
	if err != nil {
		fmt.Println(err)
		// return nil, err
	}

	filesize := fileinfo.Size()
	buffer := make([]byte, filesize)

	_, err = file.Read(buffer)
	if err != nil {
		fmt.Println(err)
		// return nil, err
	}

	tok := new(oauth2.Token)
	if err := json.Unmarshal(buffer, tok); err != nil {
		log.Fatalf("could not unmarshal token: %v", err)
	}

	// Create a Spotify authenticator with the oauth2 token.
	// If the token is expired, the oauth2 package will automatically refresh
	// so the new token is checked against the old one to see if it should be updated.
	client := spotify.NewAuthenticator("").NewClient(tok)

	newToken, err := client.Token()
	if err != nil {
		log.Fatalf("could not retrieve token from client: %v", err)
	}
	if newToken.AccessToken != tok.AccessToken {
		log.Println("got refreshed token, saving it")
	}

	_, err = json.Marshal(newToken)
	if err != nil {
		log.Fatalf("could not marshal token: %v", err)
	}

	// save new token

	// use the client to make calls that require authorization
	user, err := client.CurrentUser()
	if err != nil {
		log.Fatal(err)
	}
	log.Println("You are logged in as: ", user.ID)

	return client, nil
}
