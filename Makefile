include .env
export

.PHONY: token run build serve open-browser

token:
	go run ./cmd/get-token

run:
	go run ./cmd/spotify-yearly-discoveries

build:
	go build -o spotify-yearly-discoveries ./cmd/spotify-yearly-discoveries

serve:
	go run ./cmd/spotify-yearly-discoveries -web

open-playlist:
	python3 -m webbrowser https://open.spotify.com/playlist/$(PLAYLIST_ID)
