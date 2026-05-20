package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/jetzlstorfer/spotify-yearly-discoveries/internal/randutil"
	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
)

func buildRedirectURI() string {
	if name := os.Getenv("CODESPACE_NAME"); name != "" {
		domain := os.Getenv("GITHUB_CODESPACES_PORT_FORWARDING_DOMAIN")
		if domain == "" {
			domain = "app.github.dev"
		}
		return fmt.Sprintf("https://%s-8888.%s/callback", name, domain)
	}
	return "http://127.0.0.1:8888/callback"
}

var redirectURI = buildRedirectURI()

var tokenFile = os.Getenv("TOKEN_FILE")

var (
	auth      = spotifyauth.New(spotifyauth.WithRedirectURL(redirectURI), spotifyauth.WithScopes(spotifyauth.ScopePlaylistReadPrivate, spotifyauth.ScopePlaylistModifyPrivate, spotifyauth.ScopePlaylistModifyPublic, spotifyauth.ScopeUserLibraryRead))
	ch        = make(chan *spotify.Client)
	authState = generateState()
)

// generateState returns a cryptographically random hex string for use as an
// OAuth CSRF state parameter. Using a static string would allow an attacker who
// knows the value to forge a redirect and capture the user's token.
func generateState() string {
	return randutil.HexString(16)
}

func main() {

	url := auth.AuthURL(authState)

	// start an HTTP server
	http.HandleFunc("/callback", completeAuth)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		slog.Info("got request", "url", r.URL.String())
		http.Redirect(w, r, url, http.StatusFound)
	})
	go func() {
		if err := http.ListenAndServe(":8888", nil); err != nil {
			slog.Error("could not start HTTP server", "err", err)
			os.Exit(1)
		}
	}()

	slog.Info("please log in to Spotify by visiting the following page in your browser", "url", url)

	// wait for auth to complete
	client := <-ch

	// use the client to make calls that require authorization
	user, err := client.CurrentUser(context.Background())
	if err != nil {
		slog.Error("could not get current user", "err", err)
		os.Exit(1)
	}
	slog.Info("logged in", "displayName", user.DisplayName, "userID", user.ID)
}

func completeAuth(w http.ResponseWriter, r *http.Request) {
	tok, err := auth.Token(r.Context(), authState, r)
	if err != nil {
		http.Error(w, "Couldn't get token", http.StatusForbidden)
		slog.Error("could not get token", "err", err)
		os.Exit(1)
	}
	slog.Info("token retrieved")
	if st := r.FormValue("state"); st != authState {
		http.NotFound(w, r)
		slog.Error("state mismatch", "got", st, "want", authState)
		os.Exit(1)
	}

	tokenBytes, err := json.Marshal(tok)
	if err != nil {
		slog.Error("could not marshal token", "err", err)
		os.Exit(1)
	}

	err = os.WriteFile(tokenFile, tokenBytes, 0600)
	if err != nil {
		slog.Error("could not write file", "err", err)
		os.Exit(1)
	}

	// use the token to get an authenticated client
	httpClient := spotifyauth.New().Client(context.Background(), tok)
	client := spotify.New(httpClient)
	fmt.Fprintf(w, "Login Completed!")
	ch <- client
}
