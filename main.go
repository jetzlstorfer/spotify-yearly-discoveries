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
const logFile = "log.txt"
const year = "2020"
const tokenFile = "mytoken.txt"
const yearsPlaylistID = "2D0NidVJbZfnR4wmvYSRiA"

var (
	auth  = spotify.NewAuthenticator(redirectURI, spotify.ScopePlaylistReadPrivate)
	ch    = make(chan *spotify.Client)
	state = "abc123"
)

func main() {

	log.Println("Starting program...")

	logFile, err := os.OpenFile(logFile, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0666)
	if err != nil {
		panic(err)
	}
	mw := io.MultiWriter(os.Stdout, logFile)
	log.SetOutput(mw)

	fmt.Println("SPOTIFY_ID: " + os.Getenv(("SPOTIFY_ID")))
	client, _ := verifyLogin()

	var offset int = 0
	var limit int = 50

	var playlistsToConsider []spotify.SimplePlaylist

	var totalPlaylists int
	for p := 1; ; p++ {

		opt := spotify.Options{Limit: &limit, Offset: &offset}

		page, err := client.CurrentUsersPlaylistsOpt(&opt)
		totalPlaylists = page.Total
		if err != nil {
			log.Fatalf("couldn't get playlists: %v", err)
		}

		for _, playlist := range page.Playlists {
			if page.Next != "" {

			}
			// log.Println(playlist.Name)
			if strings.Contains(playlist.Name, year) && playlist.ID != yearsPlaylistID {
				playlistsToConsider = append(playlistsToConsider, playlist)
			}
		}
		offset = p * limit
		if page.Next == "" {
			break
		}
	}

	log.Println("Totally found playlists: " + strconv.Itoa(totalPlaylists))

	log.Println("Number of playlists with " + year + " in title: " + strconv.Itoa(len(playlistsToConsider)))
	var yearlyDiscovery []spotify.ID
	totalAdded := 0
	for _, playlist := range playlistsToConsider {
		//	log.Println("Scaning playlist: " + playlist.Name)
		// tracks := playlist.Tracks

		page, _ := client.GetPlaylistTracks(playlist.ID) // client.GetPlaylistTracks("37i9dQZF1EM7kiO6eeKyiv") //client.GetPlaylistTracks(playlist.ID)
		trackLimit := 50
		var tracksToCheck []spotify.ID
		fmt.Println(len(page.Tracks))
		for i, track := range page.Tracks {
			fmt.Println("i=" + strconv.Itoa(i))
			// check if track is from YEAR
			if trackIsFromYear((track)) {

				log.Println("Song matching " + year + " found: " + track.Track.String())

				tracksToCheck = append(tracksToCheck, track.Track.ID)
				log.Println("size of collection of tracks to check: " + strconv.Itoa(len(tracksToCheck)))

				log.Println("i=" + strconv.Itoa(i) + ", len(page.Tracks)=" + strconv.Itoa(len(page.Tracks)))

				if len(tracksToCheck) >= trackLimit {
					// check if song is added to users library
					log.Println("size of collection of tracks to check that is added: " + strconv.Itoa(len(tracksToCheck)))
					isAdded, err := client.UserHasTracks(tracksToCheck...)
					if err != nil {
						log.Fatalln(err)
					}
					for j, added := range isAdded {

						if added {
							log.Println("song was saved to users library, saving it to collection")
							totalAdded++
							// save it to be added to yearly discovery playlist
							yearlyDiscovery = append(yearlyDiscovery, tracksToCheck[j])
						} else {
							log.Println("song not saved to users library, skipping it")
						}
					}
					tracksToCheck = nil

				}

			}
			if i+1 >= len(page.Tracks) {
				// check if song is added to users library
				log.Println("size of collection of tracks to check that is added: " + strconv.Itoa(len(tracksToCheck)))
				isAdded, err := client.UserHasTracks(tracksToCheck...)
				if err != nil {
					log.Fatalln(err)
				}
				for j, added := range isAdded {

					if added {
						log.Println("song was saved to users library, saving it to collection")
						totalAdded++
						// save it to be added to yearly discovery playlist
						yearlyDiscovery = append(yearlyDiscovery, tracksToCheck[j])
					} else {
						log.Println("song not saved to users library, skipping it")
					}
				}
				tracksToCheck = nil

			}

		}

	}

	log.Println("Songs discovered: " + strconv.Itoa(len(yearlyDiscovery)))
	log.Println(totalAdded)

	// removing duplicate values
	yearlyDiscovery = removeDuplicateValues(yearlyDiscovery)
	log.Println("Songs discovered (unique): " + strconv.Itoa(len(yearlyDiscovery)))

	// empty playlist
	client.ReplacePlaylistTracks(yearsPlaylistID)
	if err != nil {
		log.Fatalln("could not empty playlist: " + err.Error())
	}

	// add tracks
	log.Println("adding discovered songs to playlist...")
	err = addTracksToPlaylist(client, yearlyDiscovery)
	if err != nil {
		log.Fatalln("Could not add songs to playlist: " + err.Error())
	}
	log.Println(strconv.Itoa(len(yearlyDiscovery)) + " songs added new playlist")

}

// adding songs to playlist, 100 songs each Api call
func addTracksToPlaylist(client spotify.Client, tracks []spotify.ID) error {
	var limit = 100
	for i := 0; i < len(tracks); i += limit {
		tracks := tracks[i:min(i+limit, len(tracks))]
		_, err := client.AddTracksToPlaylist(yearsPlaylistID, tracks...)
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
