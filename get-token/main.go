package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
)

const redirectURI = "http://localhost:8888/callback"

var tokenFile = os.Getenv("TOKEN_FILE")

var (
	auth  = spotifyauth.New(spotifyauth.WithRedirectURL(redirectURI), spotifyauth.WithScopes(spotifyauth.ScopePlaylistReadPrivate, spotifyauth.ScopePlaylistModifyPrivate, spotifyauth.ScopePlaylistModifyPublic, spotifyauth.ScopeUserLibraryRead))
	ch    = make(chan *spotify.Client)
	state = "myCrazyState"
)

func main() {

	// start an HTTP server
	http.HandleFunc("/callback", completeAuth)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Println("Got request for:", r.URL.String())
	})
	go func() {
		if err := http.ListenAndServe(":8888", nil); err != nil {
			log.Fatal(err)
		}
	}()

	url := auth.AuthURL(state)
	log.Println("Please log in to Spotify by visiting the following page in your browser:", url)

	// wait for auth to complete
	client := <-ch

	// use the client to make calls that require authorization
	user, err := client.CurrentUser(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("You are logged in as: %s (%s)", user.DisplayName, user.ID)
}

func completeAuth(w http.ResponseWriter, r *http.Request) {
	tok, err := auth.Token(r.Context(), state, r)
	if err != nil {
		http.Error(w, "Couldn't get token", http.StatusForbidden)
		log.Fatal(err)
	}
	log.Println("token retrieved")
	if st := r.FormValue("state"); st != state {
		http.NotFound(w, r)
		log.Fatalf("State mismatch: %s != %s\n", st, state)
	}

	btys, err := json.Marshal(tok)
	if err != nil {
		log.Fatalf("could not marshal token: %v", err)
	}

	err = os.WriteFile(tokenFile, btys, 0644)
	if err != nil {
		log.Fatalf("could not write file: %v", err)
	}

	// use the token to get an authenticated client
	httpClient := spotifyauth.New().Client(context.Background(), tok)
	client := spotify.New(httpClient)
	fmt.Fprintf(w, "Login Completed!")
	ch <- client
}
